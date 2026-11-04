//! A glyph outline shares the foreground layout so highlighting never changes wrapping.
use iced::advanced::{Layout, Renderer as _, Widget, layout, mouse, renderer, widget};
use iced::{Element, Length, Rectangle, Size, Theme, Vector};

pub fn outlined<'a, Message: 'a>(
    foreground: impl Into<Element<'a, Message>>,
    outline: impl Into<Element<'a, Message>>,
    radius: f32,
) -> Element<'a, Message> {
    Element::new(Outlined {
        foreground: foreground.into(),
        outline: outline.into(),
        radius,
        emphasis: None,
    })
}

pub fn emphasized<'a, Message: 'a>(
    foreground: impl Into<Element<'a, Message>>,
    outline: impl Into<Element<'a, Message>>,
    emphasis: impl Into<Element<'a, Message>>,
) -> Element<'a, Message> {
    Element::new(Outlined {
        foreground: foreground.into(),
        outline: outline.into(),
        radius: 1.05,
        emphasis: Some(emphasis.into()),
    })
}

struct Outlined<'a, Message> {
    foreground: Element<'a, Message>,
    outline: Element<'a, Message>,
    radius: f32,
    emphasis: Option<Element<'a, Message>>,
}

impl<Message> Widget<Message, Theme, iced::Renderer> for Outlined<'_, Message> {
    fn size(&self) -> Size<Length> {
        self.foreground.as_widget().size()
    }

    fn children(&self) -> Vec<widget::Tree> {
        let mut children = vec![
            widget::Tree::new(&self.foreground),
            widget::Tree::new(&self.outline),
        ];
        if let Some(emphasis) = &self.emphasis {
            children.push(widget::Tree::new(emphasis));
        }
        children
    }

    fn diff(&self, tree: &mut widget::Tree) {
        let mut children = vec![&self.foreground, &self.outline];
        children.extend(self.emphasis.iter());
        tree.diff_children(&children);
    }

    fn layout(
        &mut self,
        tree: &mut widget::Tree,
        renderer: &iced::Renderer,
        limits: &layout::Limits,
    ) -> layout::Node {
        let foreground =
            self.foreground
                .as_widget_mut()
                .layout(&mut tree.children[0], renderer, limits);
        let outline = self
            .outline
            .as_widget_mut()
            .layout(&mut tree.children[1], renderer, limits);
        let size = foreground.size();
        let mut children = vec![foreground, outline];
        if let Some(emphasis) = &mut self.emphasis {
            children.push(
                emphasis
                    .as_widget_mut()
                    .layout(&mut tree.children[2], renderer, limits),
            );
        }
        layout::Node::with_children(size, children)
    }

    fn draw(
        &self,
        tree: &widget::Tree,
        renderer: &mut iced::Renderer,
        theme: &Theme,
        style: &renderer::Style,
        layout: Layout<'_>,
        cursor: mouse::Cursor,
        viewport: &Rectangle,
    ) {
        let mut children = layout.children();
        let foreground = children.next().unwrap();
        let outline = children.next().unwrap();
        // Render around the glyph rather than offsetting its layout box or baseline.
        for (x, y) in [
            (-1.0, 0.0),
            (1.0, 0.0),
            (0.0, -1.0),
            (0.0, 1.0),
            (-0.7, -0.7),
            (0.7, -0.7),
            (-0.7, 0.7),
            (0.7, 0.7),
        ] {
            renderer.with_translation(Vector::new(x * self.radius, y * self.radius), |renderer| {
                self.outline.as_widget().draw(
                    &tree.children[1],
                    renderer,
                    theme,
                    style,
                    outline,
                    mouse::Cursor::Unavailable,
                    viewport,
                );
            });
        }
        if let (Some(emphasis), Some(layout)) = (&self.emphasis, children.next()) {
            for radius in [0.4, 2.0] {
                for (x, y) in [(-1.0, 0.0), (1.0, 0.0), (0.0, -1.0), (0.0, 1.0)] {
                    renderer.with_translation(Vector::new(x * radius, y * radius), |renderer| {
                        emphasis.as_widget().draw(
                            &tree.children[2],
                            renderer,
                            theme,
                            style,
                            layout,
                            mouse::Cursor::Unavailable,
                            viewport,
                        );
                    });
                }
            }
        }
        self.foreground.as_widget().draw(
            &tree.children[0],
            renderer,
            theme,
            style,
            foreground,
            cursor,
            viewport,
        );
    }
}
