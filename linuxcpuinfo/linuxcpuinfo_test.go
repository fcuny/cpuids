package linuxcpuinfo

import (
	"os"
	"testing"
)

const intelCPUInfo = `processor	: 0
vendor_id	: GenuineIntel
cpu family	: 6
model		: 143
model name	: Intel(R) Xeon(R) Platinum 8480+
stepping	: 8

processor	: 1
vendor_id	: GenuineIntel
cpu family	: 6
model		: 143
`

const armCPUInfo = `processor	: 0
BogoMIPS	: 2100.00
Features	: fp asimd evtstrm aes pmull sha1 sha2 crc32
CPU implementer	: 0x41
CPU architecture: 8
CPU variant	: 0x1
CPU part	: 0xd4f
CPU revision	: 0
`

func TestResolveIntelBlock(t *testing.T) {
	m, ok := resolve([]byte(intelCPUInfo))
	if !ok {
		t.Fatal("expected resolution")
	}
	if m.Microarch != "Sapphire Rapids" {
		t.Errorf("Microarch = %q, want Sapphire Rapids", m.Microarch)
	}
}

func TestResolveARMBlock(t *testing.T) {
	m, ok := resolve([]byte(armCPUInfo))
	if !ok {
		t.Fatal("expected resolution")
	}
	if m.Name != "Neoverse-V2" {
		t.Errorf("Name = %q, want Neoverse-V2", m.Name)
	}
}

func TestResolveUnknown(t *testing.T) {
	if _, ok := resolve([]byte("vendor_id\t: GenuineIntel\ncpu family\t: 6\nmodel\t: 254\n")); ok {
		t.Error("expected miss for unknown model")
	}
	if _, ok := resolve(nil); ok {
		t.Error("expected miss for empty input")
	}
}

func TestResolveFromCPUInfoFixture(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cpuinfo"
	if err := os.WriteFile(path, []byte(armCPUInfo), 0o644); err != nil {
		t.Fatal(err)
	}
	old := CPUInfoPath
	CPUInfoPath = path
	defer func() { CPUInfoPath = old }()

	m, ok := ResolveFromCPUInfo()
	if !ok || m.Name != "Neoverse-V2" {
		t.Errorf("ResolveFromCPUInfo() = %+v, %v", m, ok)
	}
}
