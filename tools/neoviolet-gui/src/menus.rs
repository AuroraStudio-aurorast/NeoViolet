use gpui::*;
use std::sync::OnceLock;

use crate::components;
use crate::platform::{PlatformKind, current_platform_kind};
use crate::state::AppState;

actions!(
    neoviolet_gui,
    [
        About,
        Preferences,
        QuitApp,
        ZoomIn,
        ZoomOut,
        ZoomReset,
        OpenRepository,
        OpenFile,
        ToggleDesktopLyrics,
        MinimizeWindow,
        ZoomWindow,
        HideApplication,
        HideOtherApplications,
        ShowAllApplications,
    ]
);

pub static CLI_VER: OnceLock<String> = OnceLock::new();
pub static GUI_VER: &str = env!("CARGO_PKG_VERSION");

// ── Menu data model ──
//
// A menu is described as data, one layout per platform, and everything else —
// the keys that get bound, the rows the native menu bar installs, and the rows
// a menu drawn by the app itself shows — is read back out of that layout. A
// shortcut is therefore written once, next to the row it belongs to, and cannot
// drift from the binding it advertises.

/// What a menu row does, named apart from the `actions!` types so a layout can
/// be written as data and still reach the right gpui action through
/// [`boxed_action`].
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MenuAction {
    About,
    Preferences,
    Quit,
    ZoomIn,
    ZoomOut,
    ZoomReset,
    OpenRepository,
    OpenFile,
    ToggleDesktopLyrics,
    MinimizeWindow,
    ZoomWindow,
    HideApplication,
    HideOtherApplications,
    ShowAllApplications,
}

pub fn action_id(action: MenuAction) -> &'static str {
    match action {
        MenuAction::About => "about",
        MenuAction::Preferences => "preferences",
        MenuAction::Quit => "quit",
        MenuAction::ZoomIn => "zoom-in",
        MenuAction::ZoomOut => "zoom-out",
        MenuAction::ZoomReset => "zoom-reset",
        MenuAction::OpenRepository => "open-repository",
        MenuAction::OpenFile => "open-file",
        MenuAction::ToggleDesktopLyrics => "toggle-desktop-lyrics",
        MenuAction::MinimizeWindow => "minimize-window",
        MenuAction::ZoomWindow => "zoom-window",
        MenuAction::HideApplication => "hide-application",
        MenuAction::HideOtherApplications => "hide-other-applications",
        MenuAction::ShowAllApplications => "show-all-applications",
    }
}

pub fn action_from_id(id: &str) -> Option<MenuAction> {
    Some(match id {
        "about" => MenuAction::About,
        "preferences" => MenuAction::Preferences,
        "quit" => MenuAction::Quit,
        "zoom-in" => MenuAction::ZoomIn,
        "zoom-out" => MenuAction::ZoomOut,
        "zoom-reset" => MenuAction::ZoomReset,
        "open-repository" => MenuAction::OpenRepository,
        "open-file" => MenuAction::OpenFile,
        "toggle-desktop-lyrics" => MenuAction::ToggleDesktopLyrics,
        "minimize-window" => MenuAction::MinimizeWindow,
        "zoom-window" => MenuAction::ZoomWindow,
        "hide-application" => MenuAction::HideApplication,
        "hide-other-applications" => MenuAction::HideOtherApplications,
        "show-all-applications" => MenuAction::ShowAllApplications,
        _ => return None,
    })
}

/// One row of a menu.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum MenuEntry {
    /// Runs `action`, titled `label`, showing `shortcut` when it has one.
    Action {
        action: MenuAction,
        label: &'static str,
        shortcut: Option<&'static str>,
    },
    Separator,
    /// macOS fills its Services submenu itself.
    Services,
}

/// A named menu and its rows, in order.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct MenuGroup {
    pub title: &'static str,
    pub entries: Vec<MenuEntry>,
}

/// A row that runs `action`; see [`MenuEntry::Action`].
fn entry(action: MenuAction, label: &'static str, shortcut: Option<&'static str>) -> MenuEntry {
    MenuEntry::Action {
        action,
        label,
        shortcut,
    }
}

/// The menu bar this platform shows.
///
/// Each platform spells its own layout: macOS keeps the application menu and the
/// Window menu the system expects, while Windows and Linux share a File / Edit /
/// View / Help bar whose shortcuts read as the keys those platforms press. The
/// app menu is macOS-only, so the Windows and Linux menus are what the app draws
/// for itself — gpui installs no native menu bar there.
///
/// `secondary` is gpui's own spelling for Command on macOS and Control elsewhere;
/// it appears only on the macOS side, where a row shows the same key the keymap
/// binds.
pub fn layout(kind: PlatformKind) -> Vec<MenuGroup> {
    match kind {
        PlatformKind::MacOs => vec![
            MenuGroup {
                title: "NeoViolet GUI",
                entries: vec![
                    entry(MenuAction::About, "About NeoViolet GUI", None),
                    MenuEntry::Separator,
                    entry(MenuAction::Preferences, "Preferences…", Some("secondary-,")),
                    MenuEntry::Separator,
                    MenuEntry::Services,
                    MenuEntry::Separator,
                    entry(
                        MenuAction::HideApplication,
                        "Hide NeoViolet GUI",
                        Some("cmd-h"),
                    ),
                    entry(
                        MenuAction::HideOtherApplications,
                        "Hide Others",
                        Some("cmd-alt-h"),
                    ),
                    entry(MenuAction::ShowAllApplications, "Show All", None),
                    MenuEntry::Separator,
                    entry(MenuAction::Quit, "Quit", Some("secondary-q")),
                ],
            },
            MenuGroup {
                title: "File",
                entries: vec![entry(
                    MenuAction::OpenFile,
                    "Open File…",
                    Some("secondary-o"),
                )],
            },
            MenuGroup {
                title: "View",
                entries: vec![
                    entry(MenuAction::ZoomIn, "Zoom In", Some("secondary-+")),
                    entry(MenuAction::ZoomOut, "Zoom Out", Some("secondary--")),
                    entry(MenuAction::ZoomReset, "Actual Size", Some("secondary-0")),
                    entry(
                        MenuAction::ToggleDesktopLyrics,
                        "Toggle Desktop Lyrics",
                        Some("secondary-shift-l"),
                    ),
                ],
            },
            MenuGroup {
                title: "Window",
                entries: vec![
                    entry(MenuAction::MinimizeWindow, "Minimize", Some("cmd-m")),
                    entry(MenuAction::ZoomWindow, "Zoom", None),
                ],
            },
            MenuGroup {
                title: "Help",
                entries: vec![entry(MenuAction::OpenRepository, "GitHub Repository", None)],
            },
        ],
        PlatformKind::Windows | PlatformKind::Linux => {
            let quit_label = if kind == PlatformKind::Windows {
                "Exit"
            } else {
                "Quit"
            };
            vec![
                MenuGroup {
                    title: "File",
                    entries: vec![
                        entry(MenuAction::OpenFile, "Open File…", Some("ctrl-o")),
                        MenuEntry::Separator,
                        entry(MenuAction::Quit, quit_label, Some("ctrl-q")),
                    ],
                },
                MenuGroup {
                    title: "Edit",
                    entries: vec![entry(
                        MenuAction::Preferences,
                        "Preferences…",
                        Some("ctrl-,"),
                    )],
                },
                MenuGroup {
                    title: "View",
                    entries: vec![
                        entry(MenuAction::ZoomIn, "Zoom In", Some("ctrl-+")),
                        entry(MenuAction::ZoomOut, "Zoom Out", Some("ctrl--")),
                        entry(MenuAction::ZoomReset, "Actual Size", Some("ctrl-0")),
                        entry(
                            MenuAction::ToggleDesktopLyrics,
                            "Toggle Desktop Lyrics",
                            Some("ctrl-shift-l"),
                        ),
                    ],
                },
                MenuGroup {
                    title: "Help",
                    entries: vec![
                        entry(
                            MenuAction::OpenRepository,
                            "GitHub Repository",
                            Some("ctrl-shift-r"),
                        ),
                        MenuEntry::Separator,
                        entry(MenuAction::About, "About NeoViolet", Some("ctrl-shift-a")),
                    ],
                },
            ]
        }
    }
}

/// Every shortcut `kind`'s menu displays, with the key it shows — read out of
/// [`layout`]. The keymap binds these plus the platform aliases in
/// [`platform_aliases`], which are deliberately not menu rows.
pub fn keybindings(kind: PlatformKind) -> Vec<(MenuAction, &'static str)> {
    layout(kind)
        .into_iter()
        .flat_map(|group| group.entries)
        .filter_map(|entry| match entry {
            MenuEntry::Action {
                action,
                shortcut: Some(shortcut),
                ..
            } => Some((action, shortcut)),
            _ => None,
        })
        .collect()
}

/// The gpui action a menu row stands for.
///
/// This is the one place a [`MenuAction`] turns into something the keymap, the
/// native menu bar and a menu drawn by the app can all hold.
pub fn boxed_action(action: MenuAction) -> Box<dyn Action> {
    match action {
        MenuAction::About => Box::new(About),
        MenuAction::Preferences => Box::new(Preferences),
        MenuAction::Quit => Box::new(QuitApp),
        MenuAction::ZoomIn => Box::new(ZoomIn),
        MenuAction::ZoomOut => Box::new(ZoomOut),
        MenuAction::ZoomReset => Box::new(ZoomReset),
        MenuAction::OpenRepository => Box::new(OpenRepository),
        MenuAction::OpenFile => Box::new(OpenFile),
        MenuAction::ToggleDesktopLyrics => Box::new(ToggleDesktopLyrics),
        MenuAction::MinimizeWindow => Box::new(MinimizeWindow),
        MenuAction::ZoomWindow => Box::new(ZoomWindow),
        MenuAction::HideApplication => Box::new(HideApplication),
        MenuAction::HideOtherApplications => Box::new(HideOtherApplications),
        MenuAction::ShowAllApplications => Box::new(ShowAllApplications),
    }
}

pub fn dispatch(action: MenuAction, cx: &mut App) {
    let action = boxed_action(action);
    cx.dispatch_action(action.as_ref());
}

/// The window a menu action should act on: the one this app opened, falling back
/// to whatever the platform reports as active.
fn window_to_control(cx: &App) -> Option<AnyWindowHandle> {
    let opened = *cx.global::<AppState>().main_window_handle.lock().unwrap();
    opened.or_else(|| cx.active_window())
}

// ── Keybindings ──

/// Keys `kind` binds on top of its menu rows.
///
/// macOS presses ⌘= for Zoom In as readily as the ⌘+ the menu advertises — ⌘+
/// needs Shift on most layouts, so the two are not the same key, and the bare ⌘=
/// has always been bound beside it. An alias is not a menu row, which is why it
/// lives here instead of in [`layout`].
fn platform_aliases(kind: PlatformKind) -> Vec<(MenuAction, &'static str)> {
    match kind {
        PlatformKind::MacOs => vec![(MenuAction::ZoomIn, "secondary-=")],
        PlatformKind::Windows | PlatformKind::Linux => Vec::new(),
    }
}

fn register_bindings(cx: &mut App) {
    let kind = current_platform_kind();

    // `KeyBinding::new` takes the concrete action type; the layout hands back a
    // runtime choice, so load the boxed action directly instead.
    let bindings = keybindings(kind)
        .into_iter()
        .chain(platform_aliases(kind))
        .map(|(action, keystrokes)| {
            KeyBinding::load(
                keystrokes,
                boxed_action(action),
                None,
                false,
                None,
                &DummyKeyboardMapper,
            )
            .expect("menu shortcuts are valid keystrokes")
        })
        .collect::<Vec<_>>();
    cx.bind_keys(bindings);
}

pub fn setup(cx: &mut App, neoviolet_path: Option<&str>) {
    // Cache CLI version asynchronously — never block startup.
    let configured_path = neoviolet_path.map(|s| s.to_string());
    let version_tx = {
        let state = cx.global::<AppState>();
        state.cli_version.clone()
    };
    std::thread::spawn(move || {
        let ver = crate::platform::fetch_cli_version(configured_path.as_deref());
        let _ = CLI_VER.set(ver.clone());
        if let Ok(mut guard) = version_tx.lock() {
            *guard = ver;
        }
    });

    // About
    cx.on_action(move |_: &About, cx: &mut App| {
        components::open_about(cx);
    });

    // Preferences — reveal config directory
    cx.on_action(|_: &Preferences, cx: &mut App| {
        let dir = crate::config::config_dir_path();
        cx.reveal_path(&dir);
    });

    // Quit — skip confirmation if process already exited
    cx.on_action(|_: &QuitApp, cx: &mut App| {
        let skip_confirm = {
            let state = cx.global::<AppState>();
            *state.show_exit_error.lock().unwrap()
        };
        if skip_confirm {
            cx.quit();
        } else {
            components::open_close(cx);
        }
    });

    // Zoom — updates AppState + live TerminalApp
    let zoom_handler = move |delta: fn(&mut u32), cx: &mut App| {
        let (new_fs, weak_opt, root_eid) = {
            let state = cx.global::<AppState>();
            let mut fs = state.font_size.lock().unwrap();
            delta(&mut fs);
            let new_fs = *fs as f32;
            let weak = state.terminal_child.lock().unwrap().clone();
            let eid = *state.root_entity_id.lock().unwrap();
            (new_fs, weak, eid)
        };
        if let Some(ref weak) = weak_opt
            && let Some(terminal) = weak.upgrade()
        {
            terminal.update(cx, |t, cx| {
                t.terminal_font_size = new_fs;
                cx.notify();
            });
        }
        if let Some(eid) = root_eid {
            cx.notify(eid);
        }
    };

    cx.on_action(move |_: &ZoomIn, cx: &mut App| {
        zoom_handler(|fs| *fs = (*fs + 2).min(48), cx);
    });

    cx.on_action(move |_: &ZoomOut, cx: &mut App| {
        zoom_handler(|fs| *fs = (*fs).saturating_sub(2).max(8), cx);
    });

    cx.on_action(move |_: &ZoomReset, cx: &mut App| {
        zoom_handler(|fs| *fs = 14, cx);
    });

    // Hide / Hide Others / Show All — the window-level half of the macOS
    // application menu; the OS owns the other half.
    cx.on_action(|_: &HideApplication, cx: &mut App| {
        cx.hide();
    });

    cx.on_action(|_: &HideOtherApplications, cx: &mut App| {
        cx.hide_other_apps();
    });

    cx.on_action(|_: &ShowAllApplications, cx: &mut App| {
        cx.unhide_other_apps();
    });

    // Window menu — act on the window we opened; `App::active_window` is nil on
    // macOS until the app has been activated, and a menu item that silently did
    // nothing would look like an unimplemented one.
    cx.on_action(|_: &MinimizeWindow, cx: &mut App| {
        if let Some(handle) = window_to_control(cx) {
            let _ = handle.update(cx, |_, window, _| window.minimize_window());
        }
    });

    cx.on_action(|_: &ZoomWindow, cx: &mut App| {
        if let Some(handle) = window_to_control(cx) {
            let _ = handle.update(cx, |_, window, _| window.zoom_window());
        }
    });

    cx.on_action(|_: &OpenRepository, _cx: &mut App| {
        if let Err(error) =
            crate::platform::open_url("https://github.com/AuroraStudio-aurorast/NeoViolet")
        {
            log::warn!("could not open the repository page: {error}");
        }
    });

    // Open File — native file picker, sends path via IPC socket.
    // The TUI's IPC server receives the message and triggers handleLoadTrack.
    cx.on_action(move |_: &OpenFile, cx: &mut App| {
        let ipc = cx.global::<AppState>().ipc.clone();

        let rx = cx.prompt_for_paths(PathPromptOptions {
            files: true,
            directories: false,
            multiple: false,
            prompt: Some("Open Audio File".into()),
        });

        cx.spawn(async move |_cx| {
            if let Ok(Ok(Some(paths))) = rx.await
                && let Some(path) = paths.into_iter().next()
                && let Err(e) = ipc.send_open(&path.to_string_lossy())
            {
                log::error!("[open-file] IPC send failed: {}", e);
            }
        })
        .detach();
    });

    // ── Toggle Desktop Lyrics (Cmd+Shift+L) ──
    cx.on_action(move |_: &ToggleDesktopLyrics, cx: &mut App| {
        let (enabled, ipc, lyrics_cfg) = {
            let state = cx.global::<AppState>();
            let mut guard = state.desktop_lyrics_enabled.lock().unwrap();
            *guard = !*guard;
            let enabled = *guard;
            let ipc = state.ipc.clone();
            let cfg = state.config.desktop_lyrics.clone();
            (enabled, ipc, cfg)
        };

        if enabled {
            // Tell CLI to start streaming lyrics
            if let Err(e) = ipc.send(&crate::ipc::IpcMessage::enable_desktop_lyrics(true)) {
                log::error!("[desktop-lyrics] IPC enable failed: {}", e);
            }

            // Open the lyrics overlay window and store its handle
            let handle_for_close = cx.global::<AppState>().lyrics_window_handle.clone();
            let window_opts = components::lyrics_window_options(&lyrics_cfg);

            let _ = cx.open_window(window_opts, move |window, cx| {
                let root = cx.new(crate::desktop_lyrics::DesktopLyricsView::new);
                // Store window handle for programmatic close via spawn loop.
                *handle_for_close.lock().unwrap() = Some(window.window_handle());
                root
            });
            log::info!("[desktop-lyrics] window opened");
        } else {
            // Tell CLI to stop streaming
            if let Err(e) = ipc.send(&crate::ipc::IpcMessage::enable_desktop_lyrics(false)) {
                log::error!("[desktop-lyrics] IPC disable failed: {}", e);
            }
            // Lyrics window closes itself on next render (enabled_flag == false → window.remove_window())
            log::info!("[desktop-lyrics] disabled, window will close on next render");
        }
    });

    register_bindings(cx);

    // A native menu bar is a macOS concept. gpui only keeps the menus handed to
    // `set_menus` in the platform's state on Windows and Linux — retrievable
    // through `get_menus()`, and installing no menu bar of their own; the app
    // draws its own Windows and Linux menus from the same layout. Only macOS is
    // therefore given the macOS layout.
    if current_platform_kind() == PlatformKind::MacOs {
        cx.set_menus(
            layout(PlatformKind::MacOs)
                .into_iter()
                .map(|group| Menu {
                    name: group.title.into(),
                    items: group
                        .entries
                        .into_iter()
                        .map(|entry| match entry {
                            // `MenuItem::action` wants a concrete action, and the
                            // row's action is chosen from the layout at runtime.
                            MenuEntry::Action { label, action, .. } => MenuItem::Action {
                                name: label.into(),
                                action: boxed_action(action),
                                os_action: None,
                                checked: false,
                            },
                            MenuEntry::Separator => MenuItem::separator(),
                            MenuEntry::Services => {
                                MenuItem::os_submenu("Services", SystemMenuType::Services)
                            }
                        })
                        .collect(),
                })
                .collect(),
        );
    }
}

#[cfg(test)]
mod tests {
    use super::{
        MenuAction, MenuEntry, MenuGroup, boxed_action, keybindings, layout, platform_aliases,
    };
    use crate::platform::PlatformKind;

    fn actions_of(group: &MenuGroup) -> Vec<MenuAction> {
        group
            .entries
            .iter()
            .filter_map(|e| match e {
                MenuEntry::Action { action, .. } => Some(*action),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn the_macos_app_menu_carries_the_standard_items() {
        let groups = layout(PlatformKind::MacOs);
        // 第 0 组是应用菜单；顺序与分隔位置都钉住
        assert_eq!(groups[0].title, "NeoViolet GUI");
        assert_eq!(
            groups[0].entries,
            vec![
                MenuEntry::Action {
                    action: MenuAction::About,
                    label: "About NeoViolet GUI",
                    shortcut: None
                },
                MenuEntry::Separator,
                MenuEntry::Action {
                    action: MenuAction::Preferences,
                    label: "Preferences…",
                    shortcut: Some("secondary-,")
                },
                MenuEntry::Separator,
                MenuEntry::Services,
                MenuEntry::Separator,
                MenuEntry::Action {
                    action: MenuAction::HideApplication,
                    label: "Hide NeoViolet GUI",
                    shortcut: Some("cmd-h")
                },
                MenuEntry::Action {
                    action: MenuAction::HideOtherApplications,
                    label: "Hide Others",
                    shortcut: Some("cmd-alt-h")
                },
                MenuEntry::Action {
                    action: MenuAction::ShowAllApplications,
                    label: "Show All",
                    shortcut: None
                },
                MenuEntry::Separator,
                MenuEntry::Action {
                    action: MenuAction::Quit,
                    label: "Quit",
                    shortcut: Some("secondary-q")
                },
            ]
        );
    }

    #[test]
    fn the_macos_window_menu_is_no_longer_empty() {
        let groups = layout(PlatformKind::MacOs);
        let window = groups
            .iter()
            .find(|g| g.title == "Window")
            .expect("macOS 必须有 Window 菜单");
        assert_eq!(
            actions_of(window),
            vec![MenuAction::MinimizeWindow, MenuAction::ZoomWindow]
        );
    }

    #[test]
    fn the_non_macos_layout_is_file_edit_view_help_without_gaps() {
        for kind in [PlatformKind::Windows, PlatformKind::Linux] {
            let groups = layout(kind);
            let titles: Vec<&str> = groups.iter().map(|g| g.title).collect();
            assert_eq!(titles, vec!["File", "Edit", "View", "Help"]);
            assert!(
                groups.iter().all(|g| !g.entries.is_empty()),
                "不允许出现空分组"
            );
            assert!(
                groups
                    .iter()
                    .all(|g| !g.entries.contains(&MenuEntry::Services)),
                "Services 只属于 macOS 应用菜单"
            );
            // About 在 Help 组、并且是最后一项
            let help = actions_of(&groups[3]);
            assert_eq!(help.last(), Some(&MenuAction::About));
            // Preferences 在 Edit 组（Windows 与 Linux 一致）
            assert_eq!(actions_of(&groups[1]), vec![MenuAction::Preferences]);
        }
    }

    #[test]
    fn platform_labels_follow_each_platform() {
        let windows_file = layout(PlatformKind::Windows)[0].entries.clone();
        let linux_file = layout(PlatformKind::Linux)[0].entries.clone();
        let label = |entries: &[MenuEntry], want: MenuAction| {
            entries
                .iter()
                .find_map(|e| match e {
                    MenuEntry::Action { action, label, .. } if *action == want => Some(*label),
                    _ => None,
                })
                .unwrap()
        };
        assert_eq!(label(&windows_file, MenuAction::Quit), "Exit");
        assert_eq!(label(&linux_file, MenuAction::Quit), "Quit");
    }

    #[test]
    fn every_action_appears_exactly_once_per_platform() {
        for kind in [
            PlatformKind::MacOs,
            PlatformKind::Windows,
            PlatformKind::Linux,
        ] {
            let mut seen: Vec<MenuAction> = layout(kind).iter().flat_map(actions_of).collect();
            let total = seen.len();
            seen.sort_by_key(|a| *a as u8);
            seen.dedup();
            assert_eq!(seen.len(), total, "{kind:?} 的布局里有重复动作");
        }
    }

    #[test]
    fn keybindings_are_derived_from_the_layout() {
        for kind in [
            PlatformKind::MacOs,
            PlatformKind::Windows,
            PlatformKind::Linux,
        ] {
            let declared: Vec<(MenuAction, &'static str)> = layout(kind)
                .iter()
                .flat_map(|g| g.entries.iter())
                .filter_map(|e| match e {
                    MenuEntry::Action {
                        action,
                        shortcut: Some(s),
                        ..
                    } => Some((*action, *s)),
                    _ => None,
                })
                .collect();
            assert_eq!(
                keybindings(kind),
                declared,
                "{kind:?} 的绑定必须与布局完全一致"
            );
        }
    }

    #[test]
    fn the_fallback_shortcuts_stay_off_macos() {
        let mac: Vec<MenuAction> = keybindings(PlatformKind::MacOs)
            .into_iter()
            .map(|(a, _)| a)
            .collect();
        assert!(
            !mac.contains(&MenuAction::About),
            "macOS 不该有 About 的兜底快捷键"
        );
        assert!(
            !mac.contains(&MenuAction::OpenRepository),
            "macOS 不该有仓库链接的兜底快捷键"
        );
        for kind in [PlatformKind::Windows, PlatformKind::Linux] {
            let keys = keybindings(kind);
            assert!(keys.contains(&(MenuAction::About, "ctrl-shift-a")));
            assert!(keys.contains(&(MenuAction::OpenRepository, "ctrl-shift-r")));
        }
    }

    #[test]
    fn the_windows_and_linux_keymaps_are_the_layout_spelled_out() {
        // Written out instead of read back from `layout`: comparing the two lists
        // again passes on a wrong key written into both of them, and this is the
        // list the keymap really binds.
        let expected = vec![
            (MenuAction::OpenFile, "ctrl-o"),
            (MenuAction::Quit, "ctrl-q"),
            (MenuAction::Preferences, "ctrl-,"),
            (MenuAction::ZoomIn, "ctrl-+"),
            (MenuAction::ZoomOut, "ctrl--"),
            (MenuAction::ZoomReset, "ctrl-0"),
            (MenuAction::ToggleDesktopLyrics, "ctrl-shift-l"),
            (MenuAction::OpenRepository, "ctrl-shift-r"),
            (MenuAction::About, "ctrl-shift-a"),
        ];
        // Linux differs from Windows only in the Quit row's label, which is not a
        // key, so both platforms bind the same list.
        for kind in [PlatformKind::Windows, PlatformKind::Linux] {
            assert_eq!(keybindings(kind), expected, "{kind:?} 的绑定列表");
        }
    }

    #[test]
    fn macos_keeps_the_bare_equal_key_as_a_hidden_zoom_in_alias() {
        // Aliases and menu rows are separate lists: an alias must not become a
        // menu row, and ⌘= must not drop out of the keymap because the layout
        // advertises ⌘+ only (the two are not the same physical key).
        assert_eq!(
            platform_aliases(PlatformKind::MacOs),
            vec![(MenuAction::ZoomIn, "secondary-=")]
        );
        assert!(platform_aliases(PlatformKind::Windows).is_empty());
        assert!(platform_aliases(PlatformKind::Linux).is_empty());
        assert!(
            !keybindings(PlatformKind::MacOs).contains(&(MenuAction::ZoomIn, "secondary-=")),
            "别名不该作为菜单行出现"
        );
    }

    #[test]
    fn every_menu_action_maps_to_its_own_gpui_action() {
        // The whole table is pinned: swapping two rows (`ZoomOut` for `ZoomReset`,
        // say) leaves the action names unique, so counting them would not notice.
        let expected = [
            (MenuAction::About, "neoviolet_gui::About"),
            (MenuAction::Preferences, "neoviolet_gui::Preferences"),
            (MenuAction::Quit, "neoviolet_gui::QuitApp"),
            (MenuAction::ZoomIn, "neoviolet_gui::ZoomIn"),
            (MenuAction::ZoomOut, "neoviolet_gui::ZoomOut"),
            (MenuAction::ZoomReset, "neoviolet_gui::ZoomReset"),
            (MenuAction::OpenRepository, "neoviolet_gui::OpenRepository"),
            (MenuAction::OpenFile, "neoviolet_gui::OpenFile"),
            (
                MenuAction::ToggleDesktopLyrics,
                "neoviolet_gui::ToggleDesktopLyrics",
            ),
            (MenuAction::MinimizeWindow, "neoviolet_gui::MinimizeWindow"),
            (MenuAction::ZoomWindow, "neoviolet_gui::ZoomWindow"),
            (
                MenuAction::HideApplication,
                "neoviolet_gui::HideApplication",
            ),
            (
                MenuAction::HideOtherApplications,
                "neoviolet_gui::HideOtherApplications",
            ),
            (
                MenuAction::ShowAllApplications,
                "neoviolet_gui::ShowAllApplications",
            ),
        ];

        let mut mapped = Vec::new();
        let mut names: Vec<&str> = Vec::new();
        for (action, want) in expected {
            assert_eq!(boxed_action(action).name(), want, "{action:?} 映射错了动作");
            mapped.push(action as u8);
            names.push(want);
        }

        let mut unique = names.clone();
        unique.sort_unstable();
        unique.dedup();
        assert_eq!(
            unique.len(),
            names.len(),
            "两个菜单动作映射到了同一个 gpui 动作"
        );

        // ... and every action the layouts name has a row here, with none left over.
        let mut declared: Vec<u8> = [
            PlatformKind::MacOs,
            PlatformKind::Windows,
            PlatformKind::Linux,
        ]
        .iter()
        .flat_map(|kind| layout(*kind))
        .flat_map(|group| group.entries)
        .filter_map(|entry| match entry {
            MenuEntry::Action { action, .. } => Some(action as u8),
            _ => None,
        })
        .collect();
        declared.sort_unstable();
        declared.dedup();
        mapped.sort_unstable();
        assert_eq!(mapped, declared, "映射表必须与布局里的动作一一对应");
    }

    #[test]
    fn window_actions_never_collide_with_terminal_control_characters() {
        for kind in [PlatformKind::Windows, PlatformKind::Linux] {
            assert!(
                !keybindings(kind).iter().any(|(action, _)| matches!(
                    action,
                    MenuAction::MinimizeWindow | MenuAction::ZoomWindow
                )),
                "{kind:?} 不应绑定 Window 菜单动作"
            );
        }
        let mac_window_actions: Vec<_> = keybindings(PlatformKind::MacOs)
            .into_iter()
            .filter(|(action, _)| {
                matches!(action, MenuAction::MinimizeWindow | MenuAction::ZoomWindow)
            })
            .collect();
        assert_eq!(
            mac_window_actions,
            vec![(MenuAction::MinimizeWindow, "cmd-m")]
        );
        assert!(
            mac_window_actions
                .iter()
                .all(|(_, key)| key.starts_with("cmd-"))
        );
    }
}
