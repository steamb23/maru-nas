package addonpkg

import (
	"debug/elf"
	"os"
)

// elfMachines는 백엔드 아키텍처의 ELF 기계 형식이다.
var elfMachines = map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}

// checkELF는 name이 arch용 64비트 리눅스 실행 파일(ELF, 실행 또는 위치 독립 실행)인지 본다.
// 스크립트(#!)는 받지 않는다. 유닛의 ExecStart가 이 파일을 바로 실행한다.
func checkELF(dst *os.Root, name, arch string) error {
	f, err := dst.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	ef, err := elf.NewFile(f)
	if err != nil {
		return fail(ReasonBackend, "%s: not an ELF executable: %v", name, err)
	}
	defer ef.Close()
	if ef.Class != elf.ELFCLASS64 || ef.Machine != elfMachines[arch] {
		return fail(ReasonBackend, "%s: not a 64-bit %s executable (machine %v)", name, arch, ef.Machine)
	}
	if ef.Type != elf.ET_EXEC && ef.Type != elf.ET_DYN {
		return fail(ReasonBackend, "%s: not an executable (type %v)", name, ef.Type)
	}
	if ef.OSABI != elf.ELFOSABI_NONE && ef.OSABI != elf.ELFOSABI_LINUX {
		return fail(ReasonBackend, "%s: not a Linux executable (OS ABI %v)", name, ef.OSABI)
	}
	return nil
}
