/*
 * SYNTHETIC fixture — not copied from any upstream file.
 * Reproduces only the C shapes the parser matches.
 */

struct id_part {
	const int id;
	const char *name;
};

struct hw_impl {
	const int id;
	const struct id_part *parts;
	const char *name;
};

static const struct id_part acme_part[] = {
	{ 0x001, "Acme-One" },
	{ 0xabc, "Acme-Nova" },
	{ -1, "unknown" },
};

static const struct id_part globex_part[] = {
	{ 0x0f0, "Globex-Zero" },
	{ -1, "unknown" },
};

static const struct hw_impl hw_implementer[] = {
	{ 0x99, acme_part, "Acme" },
	{ 0x42, globex_part, "Globex" },
	{ -1, NULL, "unknown" },
};
