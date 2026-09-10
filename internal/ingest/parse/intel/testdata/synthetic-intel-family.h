/*
 * SYNTHETIC fixture — not copied from any upstream file.
 * Shapes only: exercises both #define grammars the parser cares about.
 */
#ifndef _SYNTHETIC_INTEL_FAMILY_H
#define _SYNTHETIC_INTEL_FAMILY_H

#define IFM(_fam, _model)		VFM_MAKE(X86_VENDOR_INTEL, _fam, _model)

#define INTEL_ANY			IFM(X86_FAMILY_ANY, X86_MODEL_ANY)	/* non-numeric: skipped */

/* current IFM(fam, model) form */
#define INTEL_FICTIONAL_LAKE		IFM(6, 0x10)
#define INTEL_FICTIONAL_LAKE_L		IFM(6, 0x11)	/* trailing comment ignored */
#define INTEL_FICTIONAL_RIDGE_X		IFM(6, 0x2A)
#define INTEL_FICTIONAL_RIDGE_D		IFM(6, 0x2B)
#define INTEL_MYTHIC_MESA_N		IFM(6, 50)
#define INTEL_ANCIENT_QUARK		IFM(5, 0x09)

/* legacy INTEL_FAM6_<TOKEN> bare-number form */
#define INTEL_FAM6_LEGACY_PEAK_X	0x3F

/* duplicate family/model: first definition wins */
#define INTEL_FICTIONAL_LAKE_DUP		IFM(6, 0x10)

#endif
