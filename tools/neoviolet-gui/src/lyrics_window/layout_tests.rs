use super::*;
use iced::{Color, Font};

#[test]
fn lyrics_have_their_own_content_area_without_toolbar_layout_constraints() {
    use iced::advanced::{layout, widget};
    #[derive(Default)]
    struct Bounds(Vec<(iced::Rectangle, iced::Rectangle)>);
    impl widget::Operation for Bounds {
        fn traverse(&mut self, operate: &mut dyn FnMut(&mut dyn widget::Operation)) {
            operate(self);
        }
        fn scrollable(
            &mut self,
            _: Option<&widget::Id>,
            bounds: iced::Rectangle,
            content: iced::Rectangle,
            _: iced::Vector,
            _: &mut dyn widget::operation::Scrollable,
        ) {
            self.0.push((bounds, content));
        }
    }
    let renderer =
        iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(Font::DEFAULT, 16.0.into()));
    let preferences = Preferences::default();
    for size in [Size::new(300.0, 50.0), Size::new(880.0, 500.0)] {
        for lyric in ["Short".to_owned(), "这是一句非常长的歌词 ".repeat(60)] {
            let overlay = Overlay {
                hovered: true,
                frame: serde_json::from_value(serde_json::json!({
                    "elapsed":1,"lines":[{"time":0,"text":lyric,
                        "parts":[lyric, "A long additional translation ".repeat(60)]}]
                }))
                .unwrap(),
                ..Default::default()
            };
            let mut view = overlay.view(&preferences);
            let mut tree = widget::Tree::new(&view);
            let node =
                view.as_widget_mut()
                    .layout(&mut tree, &renderer, &layout::Limits::new(size, size));
            let mut bounds = Bounds::default();
            view.as_widget_mut().operate(
                &mut tree,
                layout::Layout::new(&node),
                &renderer,
                &mut bounds,
            );
            let [(viewport, _)] = bounds.0.as_slice() else {
                panic!("the lyrics must have one bounded viewport");
            };
            assert_eq!(
                viewport.y, 8.0,
                "lyrics must use their own window content area"
            );
            assert!(viewport.y + viewport.height <= size.height - 8.0 + 0.5);
            assert!(viewport.height > 0.0);
        }
    }
}

#[test]
fn ordinary_longer_bilingual_cues_keep_two_text_rows_while_dragging() {
    use iced::advanced::{layout, widget};
    struct Content(f32);
    impl widget::Operation for Content {
        fn traverse(&mut self, operate: &mut dyn FnMut(&mut dyn widget::Operation)) {
            operate(self);
        }
        fn scrollable(
            &mut self,
            _: Option<&widget::Id>,
            _: iced::Rectangle,
            content: iced::Rectangle,
            _: iced::Vector,
            _: &mut dyn widget::operation::Scrollable,
        ) {
            self.0 = content.height;
        }
    }
    let renderer =
        iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(Font::DEFAULT, 16.0.into()));
    let preferences = Preferences::default();
    let mut overlay = Overlay {
        dragging: true,
        hovered: true,
        ..Default::default()
    };
    for lyric in ["Short", "A longer phrase in its natural width"] {
        overlay.frame = serde_json::from_value(serde_json::json!({
            "elapsed":1,"lines":[{"time":0,"text":lyric,
                "parts":[lyric,"保持原本的字号和自然排版"]}]
        }))
        .unwrap();
        let size = overlay.display_size(&preferences);
        assert!(
            size.width < 880.0,
            "this cue fits within the normal width limit"
        );
        let mut view = overlay.view(&preferences);
        let mut tree = widget::Tree::new(&view);
        let node =
            view.as_widget_mut()
                .layout(&mut tree, &renderer, &layout::Limits::new(size, size));
        let mut content = Content(0.0);
        view.as_widget_mut().operate(
            &mut tree,
            layout::Layout::new(&node),
            &renderer,
            &mut content,
        );
        assert!(
            (60.0..70.0).contains(&content.0),
            "one 32px lyric plus its translation must stay on two rows: {}",
            content.0
        );
    }
}

#[test]
fn word_sweep_changes_pixels_without_shifting_english_or_bilingual_layout() {
    use iced::advanced::{
        layout, mouse,
        renderer::{Headless, Style},
        widget,
    };
    fn geometry(node: &layout::Node, out: &mut Vec<iced::Rectangle>) {
        out.push(node.bounds());
        for child in node.children() {
            geometry(child, out);
        }
    }
    for phrase in [
        "Let the quiet carry what we left unsaid",
        "把未说完的话，交给夜色",
    ] {
        let mut overlay = Overlay {
            frame: serde_json::from_value(serde_json::json!({
                "elapsed":0,"lines":[{"time":0,"end":8,"text":phrase,
                    "parts":[phrase,"A quieter additional line"],
                    "words":[{"time":0,"end":8,"text":phrase}]}]
            }))
            .unwrap(),
            ..Default::default()
        };
        let preferences = Preferences::default();
        let size = overlay.display_size(&preferences);
        let mut reference = None;
        let mut images = Vec::new();
        for elapsed in [0.0, 3.7, 7.999] {
            overlay.frame.elapsed = elapsed;
            assert_eq!(overlay.display_size(&preferences), size);
            let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
                Font::DEFAULT,
                16.0.into(),
            ));
            let mut view = overlay.view(&preferences);
            let mut tree = widget::Tree::new(&view);
            let node =
                view.as_widget_mut()
                    .layout(&mut tree, &renderer, &layout::Limits::new(size, size));
            let mut bounds = Vec::new();
            geometry(&node, &mut bounds);
            if let Some(reference) = &reference {
                assert_eq!(
                    &bounds, reference,
                    "word color must not alter wrapping or baselines"
                );
            } else {
                reference = Some(bounds);
            }
            let mut ui = iced_runtime::UserInterface::build(
                view,
                size,
                iced_runtime::user_interface::Cache::new(),
                &mut renderer,
            );
            ui.draw(
                &mut renderer,
                &iced::Theme::Dark,
                &Style {
                    text_color: Color::WHITE,
                },
                mouse::Cursor::Unavailable,
            );
            images.push(renderer.screenshot(
                Size::new(size.width as u32, size.height as u32),
                1.0,
                Color::TRANSPARENT,
            ));
        }
        assert_eq!(
            images[0].iter().skip(3).step_by(4).collect::<Vec<_>>(),
            images[1].iter().skip(3).step_by(4).collect::<Vec<_>>(),
            "the click-through alpha mask must not change with word progress"
        );
        assert_eq!(
            images[1].iter().skip(3).step_by(4).collect::<Vec<_>>(),
            images[2].iter().skip(3).step_by(4).collect::<Vec<_>>()
        );
        assert_ne!(
            images[0], images[1],
            "the emphasis front must advance within a word"
        );
        assert_ne!(
            images[1], images[2],
            "the completed phrase must have a different color"
        );
    }
}

#[test]
fn ordinary_bilingual_lyrics_fit_below_tooltip_space_without_scrollbars() {
    use iced::advanced::{layout, widget};
    #[derive(Default)]
    struct Bounds(Vec<(iced::Rectangle, iced::Rectangle)>);
    impl widget::Operation for Bounds {
        fn traverse(&mut self, operate: &mut dyn FnMut(&mut dyn widget::Operation)) {
            operate(self);
        }
        fn scrollable(
            &mut self,
            _: Option<&widget::Id>,
            bounds: iced::Rectangle,
            content: iced::Rectangle,
            _: iced::Vector,
            _: &mut dyn widget::operation::Scrollable,
        ) {
            self.0.push((bounds, content));
        }
    }
    let frames = [
        serde_json::json!({"elapsed":1,"lines":[
            {"time":0,"text":"把未说完的话，交给夜色",
             "parts":["把未说完的话，交给夜色","Let the quiet carry what we left unsaid."]}
        ]}),
        serde_json::json!({"elapsed":1,"lines":[
            {"time":0,"text":"First voice"},
            {"time":0,"text":"Second voice"},
            {"time":0,"text":"Third voice"}
        ]}),
        serde_json::json!({"elapsed":1,"lines":[
            {"time":0,"prefix":"Voice A: ","text":"Hello, world!",
             "words":[{"time":0,"end":0.5,"text":"Hello, "},{"time":0.7,"end":1.5,"text":"world!"}],
             "parts":["Hello, world!","A longer translation with punctuation and spaces.","另一个翻译。"]},
            {"time":2,"end":4,"text":"Next cue"}
        ]}),
    ];
    let renderer =
        iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(Font::DEFAULT, 16.0.into()));
    for frame in frames {
        let overlay = Overlay {
            frame: serde_json::from_value(frame).unwrap(),
            ..Default::default()
        };
        for font_size in [22, 32, 48] {
            let preferences = Preferences {
                font_size,
                next_line: true,
                ..Default::default()
            };
            let size = overlay.display_size(&preferences);
            let mut view = overlay.view(&preferences);
            let mut tree = widget::Tree::new(&view);
            let node =
                view.as_widget_mut()
                    .layout(&mut tree, &renderer, &layout::Limits::new(size, size));
            let mut bounds = Bounds::default();
            view.as_widget_mut().operate(
                &mut tree,
                layout::Layout::new(&node),
                &renderer,
                &mut bounds,
            );
            assert!(!bounds.0.is_empty());
            for (viewport, content) in bounds.0 {
                assert!(
                    content.height <= viewport.height + 0.5,
                    "font size {font_size}: content {} exceeds viewport {}",
                    content.height,
                    viewport.height
                );
            }
        }
    }
}

#[test]
fn short_gaps_keep_overlay_geometry_until_the_next_cue_and_clear_removes_it() {
    let mut overlay=Overlay {frame:serde_json::from_value(serde_json::json!({
        "elapsed":1.9,"lines":[
            {"time":1,"end":2,"text":"A sufficiently long line to size the window", "parts":["A sufficiently long line to size the window","Additional translation"]},
            {"time":2.15,"end":4,"text":"Next"}
        ]
    })).unwrap(),..Default::default()};
    let preferences = Preferences::default();
    let before = overlay.display_size(&preferences);
    overlay.frame.elapsed = 2.05;
    assert_eq!(overlay.display_size(&preferences), before);
    overlay.frame.elapsed = 2.15;
    assert_eq!(overlay.frame.display_lines()[0].text, "Next");
    overlay.clear();
    assert!(overlay.frame.display_lines().is_empty());
    assert_eq!(overlay.display_size(&preferences), Size::new(300.0, 50.0));
}

#[test]
fn overlay_bounds_follow_lyrics_and_optional_sublines_without_growing_on_progress() {
    let mut overlay = Overlay { frame: serde_json::from_value(serde_json::json!({
        "elapsed": 1, "lines": [{"time":0,"text":"Short line", "parts":["Short line","A second line"]}]
    })).unwrap(), ..Default::default() };
    let mut preferences = Preferences {
        additional_lines: false,
        ..Default::default()
    };
    let short = overlay.display_size(&preferences);
    preferences.additional_lines = true;
    let bilingual = overlay.display_size(&preferences);
    assert!(bilingual.height > short.height);
    overlay.frame.lines[0].parts[0] =
        "A considerably longer line with more words to read on the desktop".into();
    let long = overlay.display_size(&preferences);
    assert!(long.width > short.width);
    overlay.frame.elapsed = 2.0;
    assert_eq!(overlay.display_size(&preferences), long);
}
