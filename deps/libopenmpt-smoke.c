/*
 * Smoke test for a built libopenmpt shared library.
 *
 * Loads the library at run time, the same way the application does, then checks
 * that the expected entry points are present and that the library actually
 * decodes audio: a minimal ProTracker MOD is built in memory, the whole song is
 * rendered, and silence fails the run. "It links" is not "it works", so the
 * decode step is the point of this program.
 *
 * Usage:   libopenmpt-smoke <path to a libopenmpt shared library>
 * Exit:    0 on success, non-zero with a reason on any failed check.
 *
 * The library is only inspected through the public C API. Which dependencies it
 * carries is the caller's business: the dependency pipeline's platform scripts
 * check that separately, by reading the library's load commands.
 *
 * Portability: everything except the loader shim below is plain C99. The shim is
 * the single platform-specific part of this file; on Windows the three functions
 * become LoadLibraryA / GetProcAddress / FreeLibrary. On Linux, dlopen lives in
 * libc from glibc 2.34 on, so older toolchains need -ldl when linking.
 */

#if defined(_WIN32)
#include <windows.h>
#else
#include <dlfcn.h>
#endif

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ---- loader shim: the only platform-specific code in this file ---- */
#if defined(_WIN32)

static void *load_library(const char *path) {
	return (void *)LoadLibraryA(path);
}

static void *lookup_symbol(void *handle, const char *name) {
	return (void *)(uintptr_t)GetProcAddress((HMODULE)handle, name);
}

static void unload_library(void *handle) {
	FreeLibrary((HMODULE)handle);
}
#else

static void *load_library(const char *path) {
	return dlopen(path, RTLD_NOW | RTLD_LOCAL);
}

static void *lookup_symbol(void *handle, const char *name) {
	return dlsym(handle, name);
}

static void unload_library(void *handle) {
	dlclose(handle);
}
#endif

/* ---- libopenmpt entry points used here (the public C API) ---- */

typedef uint32_t (*get_version_fn)(void);
typedef const char *(*get_string_fn)(const char *);
typedef const char *(*get_extensions_fn)(void);
typedef void (*free_string_fn)(const char *);
typedef void *(*create_from_memory_fn)(const void *data, size_t size,
                                       void *logfunc, void *loguser,
                                       void *errfunc, void *erruser,
                                       int *error, const char **error_message,
                                       void *initial_ctl);
typedef size_t (*read_stereo_fn)(void *mod, int32_t rate, size_t count, int16_t *buf);
typedef double (*duration_fn)(void *mod);
typedef void (*destroy_fn)(void *mod);
typedef size_t (*probe_recommended_size_fn)(void);
typedef int (*probe_header_fn)(uint64_t flags, const void *data, size_t size, uint64_t filesize,
                               void *logfunc, void *loguser, void *errfunc, void *erruser,
                               int *error, const char **error_message);
typedef const char *(*get_metadata_fn)(void *mod, const char *key);
typedef int (*set_repeat_count_fn)(void *mod, int32_t count);
typedef double (*set_position_seconds_fn)(void *mod, double seconds);

static void *need_symbol(void *handle, const char *name) {
	void *symbol = lookup_symbol(handle, name);
	if (!symbol) {
		fprintf(stderr, "FAIL: library does not export %s\n", name);
		exit(1);
	}
	return symbol;
}

/*
 * Smallest playable MOD: a 1084-byte header, one pattern (1024 bytes) and one
 * 64-byte sample. Header layout: 0-19 title, 20-49 sample 1 header (22-23
 * length in words, 25 volume, 28-29 loop length), 950 song length, 952-1079
 * order table, 1080-1083 the 'M.K.' magic.
 */
static size_t build_minimal_mod(uint8_t *out) {
	memset(out, 0, 1084 + 1024 + 64);
	memcpy(out, "smoke", 5);
	out[20 + 22] = 0x00;
	out[20 + 23] = 0x20; /* sample 1 length = 32 words = 64 bytes */
	out[20 + 25] = 64;   /* volume */
	/* Loop start and loop length stay 0: the sample does not loop. */
	out[950] = 1;        /* song length = 1 pattern */
	memcpy(out + 1080, "M.K.", 4);

	/*
	 * Row 0, channel 0: play sample 1 at period 0x01AC (C-3). A MOD splits the
	 * sample number across two nibbles: the high nibble goes in byte 0, the low
	 * nibble in the HIGH nibble of byte 2 (byte 2's low nibble is the effect
	 * number). Putting it in the wrong nibble yields sample 0, and the module
	 * still parses while rendering pure silence - which is exactly what this
	 * smoke test exists to catch.
	 */
	uint8_t *pattern = out + 1084;
	pattern[0] = 0x01; /* sample number high nibble (0) | period high nibble (1) */
	pattern[1] = 0xAC; /* period low byte -> period 0x01AC */
	pattern[2] = 0x10; /* sample number low nibble (1) | effect (0) */

	/* Signed 8-bit sample data: a square wave, so the render cannot be silent. */
	uint8_t *sample = out + 1084 + 1024;
	for (int i = 0; i < 64; i++) {
		sample[i] = (uint8_t)(i < 32 ? 100 : 156); /* 156 == -100 */
	}
	return 1084 + 1024 + 64;
}

static size_t count_fields(const char *list) {
	size_t n = 0;
	int in_field = 0;
	for (const char *p = list; *p; p++) {
		if (*p == ';') {
			in_field = 0;
		} else if (!in_field) {
			in_field = 1;
			n++;
		}
	}
	return n;
}

int main(int argc, char **argv) {
	if (argc != 2) {
		fprintf(stderr, "usage: %s <path to a libopenmpt shared library>\n", argv[0]);
		return 2;
	}

	void *handle = load_library(argv[1]);
	if (!handle) {
		fprintf(stderr, "FAIL: cannot load %s\n", argv[1]);
		return 1;
	}

	get_version_fn get_version = (get_version_fn)need_symbol(handle, "openmpt_get_library_version");
	get_string_fn get_string = (get_string_fn)need_symbol(handle, "openmpt_get_string");
	get_extensions_fn get_extensions = (get_extensions_fn)need_symbol(handle, "openmpt_get_supported_extensions");
	free_string_fn free_string = (free_string_fn)need_symbol(handle, "openmpt_free_string");
	create_from_memory_fn create_module = (create_from_memory_fn)need_symbol(handle, "openmpt_module_create_from_memory2");
	read_stereo_fn read_stereo = (read_stereo_fn)need_symbol(handle, "openmpt_module_read_interleaved_stereo");
	duration_fn get_duration = (duration_fn)need_symbol(handle, "openmpt_module_get_duration_seconds");
	destroy_fn destroy_module = (destroy_fn)need_symbol(handle, "openmpt_module_destroy");
	void *logfunc = need_symbol(handle, "openmpt_log_func_silent");
	void *errfunc = need_symbol(handle, "openmpt_error_func_ignore");

	/*
	 * The five below are not needed by this render loop, but the application
	 * binds the whole C API with purego, which resolves every symbol up front and
	 * gives up on the library if a single one is missing. Pinning them here keeps
	 * that contract in the dependency gate rather than discovering it in the app.
	 */
	probe_recommended_size_fn probe_size =
	    (probe_recommended_size_fn)need_symbol(handle, "openmpt_probe_file_header_get_recommended_size");
	probe_header_fn probe_header =
	    (probe_header_fn)need_symbol(handle, "openmpt_probe_file_header");
	get_metadata_fn get_metadata =
	    (get_metadata_fn)need_symbol(handle, "openmpt_module_get_metadata");
	set_repeat_count_fn set_repeat_count =
	    (set_repeat_count_fn)need_symbol(handle, "openmpt_module_set_repeat_count");
	set_position_seconds_fn set_position_seconds =
	    (set_position_seconds_fn)need_symbol(handle, "openmpt_module_set_position_seconds");

	uint32_t version = get_version();
	const char *features = get_string("library_features");
	const char *extensions = get_extensions();
	printf("library_version : %u.%u.%u\n",
	       (version >> 24) & 0xffu, (version >> 16) & 0xffu, version & 0xffu);
	printf("library_features: %s\n", features ? features : "(null)");
	printf("extensions      : %zu entries\n", extensions ? count_fields(extensions) : 0);

	int failed = 0;
	if (!extensions || !strstr(extensions, "mod") || !strstr(extensions, "mptm")) {
		fprintf(stderr, "FAIL: extension list is missing mod/mptm\n");
		failed = 1;
	}
	/* The strings belong to the library and must be released with its own free. */
	if (features) {
		free_string(features);
	}
	if (extensions) {
		free_string(extensions);
	}
	if (failed) {
		unload_library(handle);
		return 1;
	}

	uint8_t *module_data = malloc(1084 + 1024 + 64);
	if (!module_data) {
		fprintf(stderr, "FAIL: out of memory\n");
		unload_library(handle);
		return 1;
	}
	size_t module_size = build_minimal_mod(module_data);

	/* Detection runs before decoding in the application, so check it here. */
	size_t recommended = probe_size();
	printf("probe recommend : %zu bytes\n", recommended);
	if (recommended == 0) {
		fprintf(stderr, "FAIL: openmpt_probe_file_header_get_recommended_size returned 0\n");
		free(module_data);
		unload_library(handle);
		return 1;
	}
	int probe_error = 0;
	const char *probe_error_message = NULL;
	/* 3 = OPENMPT_PROBE_FILE_HEADER_FLAGS_DEFAULT (modules | containers). */
	int probed = probe_header(3ull, module_data, module_size, module_size,
	                          NULL, NULL, NULL, NULL, &probe_error, &probe_error_message);
	if (probed != 1) {
		fprintf(stderr, "FAIL: openmpt_probe_file_header did not recognise the test module"
		                " (result=%d error=%d %s)\n",
		        probed, probe_error, probe_error_message ? probe_error_message : "");
		free(module_data);
		unload_library(handle);
		return 1;
	}

	int error = 0;
	const char *error_message = NULL;
	void *mod = create_module(module_data, module_size, logfunc, NULL, errfunc, NULL,
	                          &error, &error_message, NULL);
	if (!mod) {
		fprintf(stderr, "FAIL: cannot load the test module (error=%d %s)\n",
		        error, error_message ? error_message : "");
		free(module_data);
		unload_library(handle);
		return 1;
	}

	/* The application reads metadata; the string belongs to the library. */
	const char *title = get_metadata(mod, "title");
	printf("metadata title  : %s\n", title && *title ? title : "(empty)");
	if (title) {
		free_string(title);
	}

	const int32_t rate = 48000;
	const size_t block = 2048;
	int16_t *buf = malloc(block * 2 * sizeof(int16_t));
	if (!buf) {
		fprintf(stderr, "FAIL: out of memory\n");
		destroy_module(mod);
		free(module_data);
		unload_library(handle);
		return 1;
	}

	double seconds = get_duration(mod);
	long long total = (long long)(seconds * rate);
	long long rendered = 0;
	int peak = 0;
	long long nonzero = 0;
	while (rendered < total) {
		size_t got = read_stereo(mod, rate, block, buf);
		if (got == 0) {
			break;
		}
		for (size_t i = 0; i < got * 2; i++) {
			int amplitude = buf[i] < 0 ? -buf[i] : buf[i];
			if (amplitude > peak) {
				peak = amplitude;
			}
			if (amplitude > 0) {
				nonzero++;
			}
		}
		rendered += (long long)got;
	}
	printf("decode          : %.2fs of audio, peak %d, non-zero samples %lld\n",
	       (double)rendered / rate, peak, nonzero);

	/*
	 * Seek and repeat-count run after the measurement above: the test module has
	 * its only note in row 0, so seeking past it would leave nothing to decode.
	 */
	if (set_repeat_count(mod, 0) != 1) {
		fprintf(stderr, "FAIL: openmpt_module_set_repeat_count(0) did not report success\n");
		destroy_module(mod);
		free(buf);
		free(module_data);
		unload_library(handle);
		return 1;
	}
	double position = set_position_seconds(mod, 1.0);
	printf("seek            : set_position_seconds(1.0) -> %.2fs\n", position);
	if (!(position >= 0.0)) {
		fprintf(stderr, "FAIL: openmpt_module_set_position_seconds returned %f\n", position);
		destroy_module(mod);
		free(buf);
		free(module_data);
		unload_library(handle);
		return 1;
	}

	destroy_module(mod);
	free(buf);
	free(module_data);
	unload_library(handle);

	if (!(seconds > 0) || rendered <= 0 || peak == 0) {
		fprintf(stderr, "FAIL: decoded nothing usable (zero duration or pure silence)\n");
		return 1;
	}

	printf("PASS\n");
	return 0;
}
