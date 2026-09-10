/*
 * SYNTHETIC fixture — not copied from any upstream file.
 * Shapes only: exercises the #define grammar the parser cares about.
 */
#ifndef _SYNTHETIC_INTEL_FAMILY_H
#define _SYNTHETIC_INTEL_FAMILY_H

#define INTEL_FAM6_ANY			X86_MODEL_ANY	/* non-numeric: skipped */

#define INTEL_FAM6_FICTIONAL_LAKE	0x10
#define INTEL_FAM6_FICTIONAL_LAKE_L	0x11
#define INTEL_FAM6_FICTIONAL_RIDGE_X	0x2A	/* trailing comment ignored */
#define INTEL_FAM6_FICTIONAL_RIDGE_D	0x2B
#define INTEL_FAM6_MYTHIC_MESA_N		50

/* duplicate model number: first definition wins */
#define INTEL_FAM6_FICTIONAL_LAKE_DUP	0x10

#endif
