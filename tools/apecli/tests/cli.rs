//! Integration tests for the apecli decoder subprocess.
//!
//! These run the real binary and check the stdout contract that
//! `internal/audio/format/apestream/backend_apecli.go` parses: a 28-byte
//! "APEP" header followed by raw PCM. The fixtures are the same ones the Go
//! decoder tests use, so both sides of the boundary agree on what a valid
//! stream looks like.
//!
//! Header expectations are measured from the fixtures rather than derived
//! from the CLI source, so a drift in either direction is caught.

use std::path::PathBuf;
use std::process::{Command, Output};

const HEADER_SIZE: usize = 28;

/// The two APE fixtures, with the channel count each one carries.
const FIXTURES: [(&str, u16); 2] = [("test_ape.ape", 2), ("test_ape_mono.ape", 1)];

fn fixture(name: &str) -> String {
    let path: PathBuf = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../testdata")
        .join(name);
    path.to_string_lossy().into_owned()
}

fn apecli(args: &[&str]) -> Output {
    Command::new(env!("CARGO_BIN_EXE_apecli"))
        .args(args)
        .output()
        .expect("failed to run the apecli binary")
}

fn stderr_of(out: &Output) -> String {
    String::from_utf8_lossy(&out.stderr).into_owned()
}

/// The 28-byte APEP header, field by field as the Go consumer reads it.
struct Header {
    magic: [u8; 4],
    declared_size: u32,
    sample_rate: u32,
    channels: u16,
    bits_per_sample: u16,
    total_samples: u64,
    block_align: u16,
    reserved: u16,
}

impl Header {
    fn parse(bytes: &[u8]) -> Header {
        assert!(
            bytes.len() >= HEADER_SIZE,
            "stdout is shorter than the APEP header: {} bytes",
            bytes.len()
        );
        let u16_at = |o: usize| u16::from_le_bytes([bytes[o], bytes[o + 1]]);
        let u32_at =
            |o: usize| u32::from_le_bytes([bytes[o], bytes[o + 1], bytes[o + 2], bytes[o + 3]]);
        let u64_at = |o: usize| {
            u64::from_le_bytes([
                bytes[o],
                bytes[o + 1],
                bytes[o + 2],
                bytes[o + 3],
                bytes[o + 4],
                bytes[o + 5],
                bytes[o + 6],
                bytes[o + 7],
            ])
        };
        Header {
            magic: [bytes[0], bytes[1], bytes[2], bytes[3]],
            declared_size: u32_at(4),
            sample_rate: u32_at(8),
            channels: u16_at(12),
            bits_per_sample: u16_at(14),
            total_samples: u64_at(16),
            block_align: u16_at(24),
            reserved: u16_at(26),
        }
    }
}

#[test]
fn writes_a_well_formed_apep_header() {
    for (name, channels) in FIXTURES {
        let out = apecli(&[fixture(name).as_str()]);
        assert!(out.status.success(), "{name} exited with {:?}", out.status);

        let h = Header::parse(&out.stdout);
        assert_eq!(&h.magic, b"APEP", "{name}: wrong magic");
        assert_eq!(
            h.declared_size as usize, HEADER_SIZE,
            "{name}: wrong header size"
        );
        assert_eq!(h.reserved, 0, "{name}: reserved must stay zero");
        assert_eq!(h.sample_rate, 44100, "{name}: unexpected sample rate");
        assert_eq!(h.channels, channels, "{name}: unexpected channel count");
        assert_eq!(h.bits_per_sample, 16, "{name}: unexpected bit depth");
        assert!(h.total_samples > 0, "{name}: no samples declared");
    }
}

/// The single invariant the Go side relies on: the bytes after the header are
/// exactly the PCM the header promises. If the two ever disagree, the decoder
/// would read past the end of the stream or silently drop audio.
#[test]
fn pcm_length_matches_the_declared_sample_count() {
    for (name, _) in FIXTURES {
        let out = apecli(&[fixture(name).as_str()]);
        assert!(out.status.success(), "{name} exited with {:?}", out.status);

        let h = Header::parse(&out.stdout);
        let pcm = (out.stdout.len() - HEADER_SIZE) as u64;
        assert_eq!(
            pcm,
            h.total_samples * h.block_align as u64,
            "{name}: {pcm} PCM bytes do not match {} samples x {} block align",
            h.total_samples,
            h.block_align
        );
    }
}

#[test]
fn block_align_matches_the_channel_layout() {
    for (name, channels) in FIXTURES {
        let out = apecli(&[fixture(name).as_str()]);
        let h = Header::parse(&out.stdout);
        assert_eq!(
            h.block_align,
            channels * h.bits_per_sample / 8,
            "{name}: block align disagrees with the channel layout"
        );
    }
}

#[test]
fn seek_returns_the_remaining_samples() {
    let path = fixture("test_ape.ape");
    let path = path.as_str();
    let full = apecli(&[path]);
    let h = Header::parse(&full.stdout);
    let (total, align) = (h.total_samples, h.block_align as u64);

    // Seeking to 0 is the same as not seeking at all.
    for seek in [0u64, 1, 1000, total - 1] {
        let out = apecli(&["--seek", &seek.to_string(), path]);
        assert!(
            out.status.success(),
            "--seek {seek} exited with {:?}",
            out.status
        );
        let pcm = (out.stdout.len() - HEADER_SIZE) as u64;
        assert_eq!(
            pcm,
            (total - seek) * align,
            "--seek {seek}: decoded {pcm} bytes, expected {}",
            (total - seek) * align
        );
    }
}

/// A seek past the end is rejected *after* the header has already been
/// written. Pinning this records what a half-written stream looks like — 28
/// bytes and a non-zero exit — which the Go consumer must not treat as audio.
#[test]
fn seek_past_the_end_is_rejected_after_the_header() {
    let path = fixture("test_ape.ape");
    let path = path.as_str();
    let total = Header::parse(&apecli(&[path]).stdout).total_samples;

    let out = apecli(&["--seek", &(total + 1).to_string(), path]);
    assert_eq!(
        out.status.code(),
        Some(4),
        "out-of-range seek should exit 4"
    );
    assert_eq!(
        out.stdout.len(),
        HEADER_SIZE,
        "the header is written before the seek range is validated"
    );
    assert!(
        stderr_of(&out).contains("out of range"),
        "expected an out-of-range diagnostic, got: {}",
        stderr_of(&out)
    );
}

#[test]
fn usage_errors_exit_1_without_writing_anything() {
    let path = fixture("test_ape.ape");
    let cases: [&[&str]; 4] = [
        &[],
        &["--bogus"],
        &["--seek"],
        &["--seek", "not-a-number", path.as_str()],
    ];
    for args in cases {
        let out = apecli(args);
        assert_eq!(out.status.code(), Some(1), "{args:?} should exit 1");
        assert!(out.stdout.is_empty(), "{args:?} must not write to stdout");
    }
}

#[test]
fn missing_input_file_exits_2() {
    let out = apecli(&[fixture("this_file_does_not_exist.ape").as_str()]);
    assert_eq!(out.status.code(), Some(2), "a missing file should exit 2");
    assert!(out.stdout.is_empty());
}

#[test]
fn undecodable_input_exits_3() {
    // Real audio files that are not APE: the decoder must reject them before
    // any header reaches stdout.
    for name in ["test_mp3_with_metadata.mp3", "test_wav_no_metadata.wav"] {
        let path = fixture(name);
        let path = path.as_str();

        let out = apecli(&[path]);
        assert_eq!(out.status.code(), Some(3), "{name} should exit 3");
        assert!(out.stdout.is_empty(), "{name} must not write to stdout");

        // Seeking does not rescue an undecodable file; it fails even earlier.
        let out = apecli(&["--seek", "5", path]);
        assert_eq!(out.status.code(), Some(3), "--seek {name} should exit 3");
        assert!(out.stdout.is_empty());
    }
}
