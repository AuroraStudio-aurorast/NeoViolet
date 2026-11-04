use super::*;

#[test]
fn overlapping_voices_end_at_explicit_or_next_distinct_boundaries() {
    let mut frame: Frame = serde_json::from_str(
        r#"{"elapsed":12,"lines":[
            {"time":20,"end":0,"text":"last"},
            {"time":10,"end":0,"text":"voice A"},
            {"time":10,"end":14,"text":"voice B"},
            {"time":14,"end":18,"text":"next"}
        ]}"#,
    )
    .unwrap();
    let texts = |frame: &Frame| {
        frame
            .active_lines()
            .iter()
            .map(|line| line.text.clone())
            .collect::<Vec<_>>()
    };
    assert_eq!(texts(&frame), ["voice A", "voice B"]);
    frame.elapsed = 14.0;
    assert_eq!(texts(&frame), ["next"]);
    frame.elapsed = 18.0;
    assert!(texts(&frame).is_empty());
    assert_eq!(frame.next_line().unwrap().text, "last");
    frame.elapsed = 120.0;
    assert_eq!(texts(&frame), ["last"]);
    frame.elapsed = 11.0;
    assert_eq!(texts(&frame), ["voice A", "voice B"]);
}

#[test]
fn parts_replace_text_and_timings_preserve_utf8_and_whitespace() {
    let line: Line = serde_json::from_str(
        r#"{"time":1,"end":8,"text":"fallback","parts":["你 Hello 🌙","Additional","Another"],
        "prefix":"Voice A:","words":[{"time":1,"text":"你 "},{"time":3,"text":"Hello "},{"time":6,"text":"🌙"}]}"#,
    )
    .unwrap();
    assert_eq!(line.first_text(), "你 Hello 🌙");
    assert_eq!(line.additional_lines(), ["Additional", "Another"]);
    assert_eq!(
        line.fragments(4.5, |_| 1.0),
        [
            ("你", 1.0),
            (" ", 1.0),
            ("H", 1.0),
            ("e", 1.0),
            ("l", 1.0),
            ("l", 0.0),
            ("o", 0.0),
            (" ", 0.0),
            ("🌙", 0.0)
        ]
    );
    assert!(
        line.fragments(0.0, |_| 1.0)
            .iter()
            .all(|(_, intensity)| *intensity == 0.0)
    );
    assert_eq!(line.prefix, "Voice A:");
}

#[test]
fn invalid_word_tiling_or_order_falls_back_to_the_actual_line() {
    for words in [
        r#"[{"time":1,"text":"different"}]"#,
        r#"[{"time":4,"text":"Hello "},{"time":2,"text":"world"}]"#,
    ] {
        let line: Line =
            serde_json::from_str(&format!(r#"{{"text":"Hello world","words":{words}}}"#)).unwrap();
        assert_eq!(line.fragments(3.0, |_| 1.0), [("Hello world", 1.0)]);
    }
}

#[test]
fn word_sweep_respects_explicit_pauses_widths_and_rewinds() {
    let line: Line = serde_json::from_str(
        r#"{"time":1,"end":8,"text":"你 Hi!",
        "words":[{"time":1,"end":3,"text":"你 "},{"time":5,"end":7,"text":"Hi!"}]}"#,
    )
    .unwrap();
    let width = |s: &str| if s == "你" { 2.0 } else { 1.0 };
    assert_eq!(
        line.fragments(2.0, width),
        [("你", 0.75), (" ", 0.0), ("H", 0.0), ("i", 0.0), ("!", 0.0)]
    );
    assert_eq!(
        line.fragments(4.0, width),
        [("你", 1.0), (" ", 1.0), ("H", 0.0), ("i", 0.0), ("!", 0.0)]
    );
    assert_eq!(
        line.fragments(6.0, width),
        [("你", 1.0), (" ", 1.0), ("H", 1.0), ("i", 0.5), ("!", 0.0)]
    );
    assert_eq!(line.fragments(2.0, width)[0].1, 0.75);
    assert!(
        line.fragments(8.0, width)
            .iter()
            .all(|(_, intensity)| *intensity == 1.0)
    );
}

#[test]
fn sweep_keeps_combining_marks_and_emoji_as_complete_graphemes() {
    let line: Line = serde_json::from_str(
        r#"{"time":0,"end":4,"text":"é👩‍🚀!",
        "words":[{"time":0,"text":"é👩‍🚀!"}]}"#,
    )
    .unwrap();
    assert_eq!(
        line.fragments(2.0, |_| 1.0),
        [("é", 1.0), ("👩‍🚀", 0.5), ("!", 0.0)]
    );
    let unknown: Line = serde_json::from_str(
        r#"{"time":0,"text":"Hello",
        "words":[{"time":0,"text":"Hello"}]}"#,
    )
    .unwrap();
    assert!(
        unknown
            .fragments(1.0, |_| 1.0)
            .iter()
            .all(|(_, i)| *i == 1.0)
    );
}

#[test]
fn a_later_word_advances_past_an_overlapping_previous_fragment() {
    let line: Line = serde_json::from_str(
        r#"{"end":8,"text":"one two",
        "words":[{"time":0,"end":6,"text":"one "},{"time":2,"end":4,"text":"two"}]}"#,
    )
    .unwrap();
    let fragments = line.fragments(3.0, |_| 1.0);
    assert!(
        fragments[..4]
            .iter()
            .all(|(_, intensity)| *intensity == 1.0)
    );
    assert_eq!(fragments[4..], [("t", 1.0), ("w", 0.5), ("o", 0.0)]);
    let split: Line = serde_json::from_str(
        r#"{"end":8,"text":"é!",
        "words":[{"time":0,"text":"e"},{"time":2,"text":"́!"}]}"#,
    )
    .unwrap();
    assert_eq!(split.fragments(1.0, |_| 1.0), [("é!", 1.0)]);
}

#[test]
fn absent_and_null_lyric_lists_clear_previous_content_and_zero_elapsed_is_valid() {
    for json in [r#"{}"#, r#"{"lines":null}"#, r#"{"lines":[]}"#] {
        let frame: Frame = serde_json::from_str(json).unwrap();
        assert!(frame.active_lines().is_empty());
        assert!(frame.next_line().is_none());
    }
    let frame: Frame = serde_json::from_str(r#"{"lines":[{"time":0,"text":"start"}]}"#).unwrap();
    assert_eq!(frame.active_lines()[0].text, "start");
}

#[test]
fn invalid_line_intervals_do_not_replace_valid_current_or_upcoming_lines() {
    let frame: Frame = serde_json::from_str(
        r#"{"elapsed":5,"lines":[
            {"time":-1,"end":0,"text":"invalid start"},
            {"time":2,"end":1,"text":"reversed interval"},
            {"time":4,"end":-1,"text":"invalid end"},
            {"time":0,"end":0,"text":"current"},
            {"time":9,"end":12,"text":"upcoming"}
        ]}"#,
    )
    .unwrap();
    assert_eq!(frame.active_lines().len(), 1);
    assert_eq!(frame.active_lines()[0].text, "current");
    assert_eq!(frame.next_line().unwrap().text, "upcoming");
}

#[test]
fn short_interline_gaps_hold_the_previous_voices_until_the_next_line_starts() {
    let mut frame: Frame = serde_json::from_str(
        r#"{"elapsed":2.05,"lines":[
        {"time":0,"end":2,"text":"First","parts":["First","Translation"]},
        {"time":0,"end":2,"text":"Other voice","agent":"b"},
        {"time":2.15,"end":4,"text":"Next"}
    ]}"#,
    )
    .unwrap();
    assert!(frame.active_lines().is_empty());
    assert_eq!(
        frame
            .display_lines()
            .iter()
            .map(|line| line.text.as_str())
            .collect::<Vec<_>>(),
        ["First", "Other voice"]
    );
    assert_eq!(frame.display_lines()[0].additional_lines(), ["Translation"]);
    frame.elapsed = 2.15;
    assert_eq!(frame.display_lines()[0].text, "Next");
    frame.elapsed = 1.0;
    assert_eq!(frame.display_lines().len(), 2);
    frame.elapsed = 0.0;
    assert_eq!(frame.display_lines().len(), 2);
}

#[test]
fn long_gaps_intro_outro_and_empty_payloads_do_not_hold_stale_lyrics() {
    let mut frame: Frame = serde_json::from_str(
        r#"{"elapsed":2.05,"lines":[
        {"time":1,"end":2,"text":"First"},{"time":3,"end":4,"text":"Next"}
    ]}"#,
    )
    .unwrap();
    assert!(frame.display_lines().is_empty());
    frame.elapsed = 0.5;
    assert!(frame.display_lines().is_empty());
    frame.elapsed = 4.05;
    assert!(frame.display_lines().is_empty());
    frame.lines.clear();
    assert!(frame.display_lines().is_empty());
}

#[test]
fn gap_threshold_uses_the_whole_gap_and_never_revives_an_earlier_voice() {
    let mut frame: Frame = serde_json::from_str(
        r#"{"elapsed":10.05,"lines":[
        {"time":8,"end":9.9,"text":"Earlier voice"},
        {"time":9,"end":10,"text":"Last voice"},
        {"time":10.35,"end":12,"text":"Next"}
    ]}"#,
    )
    .unwrap();
    assert_eq!(
        frame
            .display_lines()
            .iter()
            .map(|line| line.text.as_str())
            .collect::<Vec<_>>(),
        ["Last voice"]
    );
    frame.lines[2].time = 10.351;
    assert!(frame.display_lines().is_empty());
    frame.elapsed = 10.3509;
    assert!(
        frame.display_lines().is_empty(),
        "do not show old text just because the next cue is close now"
    );
    frame.elapsed = 9.0;
    assert_eq!(frame.display_lines().len(), 2);
}
