//! Detection of a composing macOS input source.
//!
//! This GPUI version exposes no way to ask whether an input method (IME) wants
//! the next key, so the terminal has to answer that question itself before it
//! decides whether a printable key may be encoded for the PTY. The answer comes
//! from the Text Input Source API in HIToolbox.
//!
//! The declared symbols are the same ones GPUI's own macOS backend uses. They
//! are declared here rather than taken from a crate because the Core Foundation
//! crates already in the dependency graph exist in two incompatible versions,
//! and a third one would be pulled in for a handful of function pointers.

use std::ffi::c_void;

// ── CoreFoundation ──

type CFTypeRef = *const c_void;
type CFStringRef = *const c_void;
type CFArrayRef = *const c_void;
type CFBooleanRef = *const c_void;
type CFIndex = isize;

#[repr(C)]
struct CFRange {
    location: CFIndex,
    length: CFIndex,
}

#[link(name = "CoreFoundation", kind = "framework")]
unsafe extern "C" {
    fn CFEqual(first: CFTypeRef, second: CFTypeRef) -> u8;
    fn CFBooleanGetValue(value: CFBooleanRef) -> u8;
    fn CFRelease(value: CFTypeRef);
    fn CFArrayGetCount(array: CFArrayRef) -> CFIndex;
    fn CFArrayGetValueAtIndex(array: CFArrayRef, index: CFIndex) -> *const c_void;
    fn CFStringGetLength(string: CFStringRef) -> CFIndex;
    fn CFStringGetCharacters(string: CFStringRef, range: CFRange, buffer: *mut u16);
}

// ── HIToolbox ──
//
// HIToolbox is not a top-level framework: it lives under Carbon, which
// re-exports it, so Carbon is what the linker has to be pointed at.

#[link(name = "Carbon", kind = "framework")]
unsafe extern "C" {
    static kTISPropertyInputSourceType: CFStringRef;
    static kTISPropertyInputSourceIsASCIICapable: CFStringRef;
    static kTISPropertyInputSourceLanguages: CFStringRef;
    static kTISTypeKeyboardInputMode: CFStringRef;
    fn TISCopyCurrentKeyboardInputSource() -> CFTypeRef;
    fn TISGetInputSourceProperty(source: CFTypeRef, property: CFStringRef) -> *mut c_void;
}

// ── Detection ──

/// Whether the keyboard input source selected right now composes text.
///
/// Two independent signals are combined because neither is reliable alone: an
/// input source can be typed as an input mode but claim to be ASCII-capable
/// (third-party Chinese input methods do), and it can be typed as a plain
/// keyboard layout while still composing (Apple's own Pinyin layout). Reporting
/// a composing source when there is none only costs a round trip through the
/// input method, because it commits the character straight back through
/// `replace_text_in_range`; missing one makes composing input impossible, so the
/// signals are combined with OR rather than AND.
pub(super) fn ime_is_active() -> bool {
    unsafe {
        let source = TISCopyCurrentKeyboardInputSource();
        if source.is_null() {
            return false;
        }

        let is_input_mode = {
            let kind = TISGetInputSourceProperty(source, kTISPropertyInputSourceType);
            !kind.is_null()
                && CFEqual(kind as CFTypeRef, kTISTypeKeyboardInputMode as CFTypeRef) != 0
        };
        let is_ascii_capable = {
            let capable = TISGetInputSourceProperty(source, kTISPropertyInputSourceIsASCIICapable);
            !capable.is_null() && CFBooleanGetValue(capable as CFBooleanRef) != 0
        };
        let composes_cjk = {
            let languages = TISGetInputSourceProperty(source, kTISPropertyInputSourceLanguages);
            !languages.is_null() && languages_contain_a_cjk_script(languages as CFArrayRef)
        };

        // Balance the Create rule; both property lookups above borrow from the
        // source and must happen first.
        CFRelease(source);

        composes_cjk || (is_input_mode && !is_ascii_capable)
    }
}

/// Whether any of an input source's languages is written with a script that goes
/// through an input method.
unsafe fn languages_contain_a_cjk_script(languages: CFArrayRef) -> bool {
    let count = unsafe { CFArrayGetCount(languages) };
    (0..count).any(|index| {
        let language = unsafe { CFArrayGetValueAtIndex(languages, index) } as CFStringRef;
        unsafe { cf_string_to_string(language) }.is_some_and(|it| is_cjk_language(&it))
    })
}

unsafe fn cf_string_to_string(string: CFStringRef) -> Option<String> {
    if string.is_null() {
        return None;
    }

    let length = unsafe { CFStringGetLength(string) };
    if length <= 0 {
        return None;
    }

    let mut buffer = vec![0u16; length as usize];
    unsafe {
        CFStringGetCharacters(
            string,
            CFRange {
                location: 0,
                length,
            },
            buffer.as_mut_ptr(),
        );
    }
    Some(String::from_utf16_lossy(&buffer))
}

/// Whether a BCP-47 language tag needs an input method to be typed.
///
/// `zh` (Chinese), `ja` (Japanese), `ko` (Korean) and `yue` (Cantonese) do;
/// languages with a direct keyboard layout such as `en` or `de` do not. Only the
/// primary subtag is compared, so `zh-Hans-CN` and `zh_Hant` both match.
fn is_cjk_language(language: &str) -> bool {
    let language = language.to_ascii_lowercase();
    let primary = language.split(['-', '_']).next().unwrap_or("");
    matches!(primary, "zh" | "ja" | "ko" | "yue")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn composing_language_tags_are_recognised() {
        for tag in [
            "zh",
            "zh-Hans",
            "zh-Hant",
            "zh-Hans-CN",
            "ja",
            "ja-JP",
            "ko",
            "ko-KR",
            "yue",
        ] {
            assert!(is_cjk_language(tag), "{tag} needs an input method");
        }
    }

    #[test]
    fn directly_typed_language_tags_are_not_recognised() {
        for tag in ["en", "en-US", "de", "fr", "ru", "th", "vi", "uk", "es"] {
            assert!(!is_cjk_language(tag), "{tag} is typed directly");
        }
    }

    #[test]
    fn language_matching_ignores_case_and_separator_style() {
        assert!(is_cjk_language("ZH_HANS"));
        assert!(is_cjk_language("Ja-jp"));
        assert!(!is_cjk_language("EN_us"));
    }

    #[test]
    fn detection_reports_the_live_input_source() {
        // Reads the input source selected right now, which links the framework
        // symbols above and proves the calls are safe to make in-process. The
        // value itself depends on the machine; run with `--nocapture` to read it.
        println!("ime_is_active() = {}", ime_is_active());
    }
}
