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
