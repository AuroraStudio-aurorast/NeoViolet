use super::online_audio_url_is_valid;

#[test]
fn online_audio_url_validation_accepts_http_and_https_hosts() {
    for url in [
        "http://audio.example.test/track.mp3",
        "https://audio.example.test/track.mp3?token=signed",
        "HTTPS://audio.example.test/live",
        "https://user:password@audio.example.test/live",
    ] {
        assert!(online_audio_url_is_valid(url), "{url} should be accepted");
    }
}

#[test]
fn online_audio_url_validation_rejects_non_http_and_hostless_values() {
    for url in [
        "ftp://audio.example.test/track.mp3",
        "https:///track.mp3",
        "https://",
        "https://audio example.test/track.mp3",
        "track.mp3",
    ] {
        assert!(!online_audio_url_is_valid(url), "{url} should be rejected");
    }
}
