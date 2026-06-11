use ape_decoder::ApeDecoder;
use std::env;
use std::fs::File;
use std::io::{self, BufReader, Write};
use std::process;

const HEADER_SIZE: u32 = 28;

#[repr(C, packed)]
struct OutputHeader {
    magic: [u8; 4],         // "APEP"
    header_size: u32,       // 28
    sample_rate: u32,
    channels: u16,
    bits_per_sample: u16,
    total_samples: u64,
    block_align: u16,
    reserved: u16,
}

fn main() {
    let args: Vec<String> = env::args().collect();
    let mut seek_samples: Option<u64> = None;
    let mut input_path: Option<&str> = None;

    let mut i = 1;
    while i < args.len() {
        match args[i].as_str() {
            "--seek" => {
                i += 1;
                if i >= args.len() {
                    eprintln!("Error: --seek requires a sample count argument");
                    process::exit(1);
                }
                seek_samples = Some(args[i].parse().unwrap_or_else(|_| {
                    eprintln!("Error: invalid seek sample count: {}", args[i]);
                    process::exit(1);
                }));
            }
            arg if arg.starts_with("--") => {
                eprintln!("Error: unknown option: {}", arg);
                process::exit(1);
            }
            _ => {
                input_path = Some(&args[i]);
            }
        }
        i += 1;
    }

    let path = match input_path {
        Some(p) => p,
        None => {
            eprintln!("Usage: apecli [--seek <samples>] <input.ape>");
            process::exit(1);
        }
    };

    // Open the APE file
    let file = match File::open(path) {
        Ok(f) => f,
        Err(e) => {
            eprintln!("Error opening file '{}': {}", path, e);
            process::exit(2);
        }
    };
    let mut reader = BufReader::new(file);

    // Create the decoder and parse header
    let mut decoder = match ApeDecoder::new(&mut reader) {
        Ok(d) => d,
        Err(e) => {
            eprintln!("Error decoding '{}': {}", path, e);
            process::exit(3);
        }
    };

    let info = decoder.info();

    // Write binary header
    let header = OutputHeader {
        magic: *b"APEP",
        header_size: HEADER_SIZE.to_le(),
        sample_rate: info.sample_rate.to_le(),
        channels: info.channels.to_le(),
        bits_per_sample: info.bits_per_sample.to_le(),
        total_samples: info.total_samples.to_le(),
        block_align: info.block_align.to_le(),
        reserved: 0,
    };

    let stdout = io::stdout();
    let mut stdout_lock = stdout.lock();

    // Safety: transmute is OK here because OutputHeader is packed and Copy.
    let header_bytes: [u8; 28] = unsafe { std::mem::transmute(header) };
    if let Err(e) = stdout_lock.write_all(&header_bytes) {
        eprintln!("Error writing header to stdout: {}", e);
        process::exit(1);
    }
    if let Err(e) = stdout_lock.flush() {
        eprintln!("Error flushing header to stdout: {}", e);
        process::exit(1);
    }

    // Decode PCM data
    let pcm = match seek_samples {
        Some(sample) => {
            if sample >= info.total_samples {
                eprintln!(
                    "Error: seek sample {} out of range (total samples: {})",
                    sample, info.total_samples
                );
                process::exit(4);
            }
            match decoder.decode_range(sample, info.total_samples) {
                Ok(data) => data,
                Err(e) => {
                    eprintln!("Error decoding from sample {}: {}", sample, e);
                    process::exit(3);
                }
            }
        }
        None => match decoder.decode_all() {
            Ok(data) => data,
            Err(e) => {
                eprintln!("Error decoding '{}': {}", path, e);
                process::exit(3);
            }
        },
    };

    // Write PCM to stdout
    if let Err(e) = stdout_lock.write_all(&pcm) {
        eprintln!("Error writing PCM data to stdout: {}", e);
        process::exit(1);
    }
}