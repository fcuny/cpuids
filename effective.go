package cpuids

// The x86 CPUID leaf 1 EAX register splits the family and model across base and
// extended fields. The Linux kernel folds these together before exposing "cpu
// family" and "model" in /proc/cpuinfo, so a caller reading procfs already has
// effective values and should not call these functions. A caller reading raw
// CPUID leaves must fold them itself, and [X86Key] expects the folded result.
//
// Layout of leaf 1 EAX:
//
//	bits  3:0   stepping
//	bits  7:4   base model
//	bits 11:8   base family
//	bits 19:16  extended model
//	bits 27:20  extended family

// EffectiveFamily folds the extended-family field into the base family.
//
// The extended family is added only when the base family is 0x0F; for every
// other base family the extended-family bits are reserved and ignored.
func EffectiveFamily(baseFamily, extendedFamily int) int {
	if baseFamily == 0x0F {
		return baseFamily + extendedFamily
	}
	return baseFamily
}

// EffectiveModel folds the extended-model field into the base model.
//
// The extended model occupies the high nibble (it is shifted left by 4) and is
// applied only when the base family is 0x06 or 0x0F; for every other base
// family the extended-model bits are reserved and ignored.
func EffectiveModel(baseFamily, baseModel, extendedModel int) int {
	if baseFamily == 0x06 || baseFamily == 0x0F {
		return baseModel + (extendedModel << 4)
	}
	return baseModel
}
