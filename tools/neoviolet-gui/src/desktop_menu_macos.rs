//! Apply startup availability to native commands, including AppKit predefined items.
use objc2_app_kit::NSMenu;

pub(crate) fn set_commands(menu: &NSMenu, available: bool) {
    for heading in menu.itemArray() {
        if let Some(submenu) = heading.submenu() {
            set_submenu(&submenu, available);
        }
    }
}

fn set_submenu(menu: &NSMenu, available: bool) {
    menu.setAutoenablesItems(available);
    for item in menu.itemArray() {
        if !item.isSeparatorItem() {
            item.setEnabled(available);
        }
        if let Some(submenu) = item.submenu() {
            set_submenu(&submenu, available);
        }
    }
}

/// Gate only player commands; AppKit keeps its standard window/app commands.
pub(crate) fn set_player_commands(menu: &muda::Menu, available: bool, lyrics_enabled: bool) {
    use muda::ContextMenu;
    // SAFETY: muda owns the root menu and this runs on AppKit's main thread.
    let native = unsafe { &*menu.ns_menu().cast::<NSMenu>() };
    apply_player_commands(menu.items(), available, lyrics_enabled, Some(native));
}

fn apply_player_commands(
    items: Vec<muda::MenuItemKind>,
    available: bool,
    lyrics_enabled: bool,
    native: Option<&NSMenu>,
) {
    use muda::MenuItemKind;
    for item in items {
        match item {
            MenuItemKind::Submenu(menu) => {
                let children = menu.items();
                // A muda Submenu's own NSMenu is its context-menu copy. Use the
                // copy attached to this root menu; AppKit may insert other
                // headings, so match the stable English title rather than index.
                let attached = native.and_then(|parent| {
                    parent.itemArray().iter().find_map(|heading| {
                        if heading.title().to_string() == menu.text() {
                            heading.submenu()
                        } else {
                            None
                        }
                    })
                });
                if children.iter().any(|item| {
                    matches!(
                        item.id().as_ref(),
                        "open"
                            | "lyrics_settings"
                            | "unlock_lyrics"
                            | "desktop_lyrics"
                            | "reset_lyrics_position"
                    )
                }) && let Some(attached) = &attached
                {
                    attached.setAutoenablesItems(false);
                }
                apply_player_commands(children, available, lyrics_enabled, attached.as_deref())
            }
            MenuItemKind::MenuItem(item) => match item.id().as_ref() {
                "open" | "lyrics_settings" | "reset_lyrics_position" => item.set_enabled(available),
                "unlock_lyrics" => item.set_enabled(available && lyrics_enabled),
                _ => {}
            },
            MenuItemKind::Check(item) if item.id().as_ref() == "desktop_lyrics" => {
                item.set_enabled(available)
            }
            _ => {}
        }
    }
}
