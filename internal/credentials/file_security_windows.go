//go:build windows

package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileAllAccess windows.ACCESS_MASK = 0x001F01FF
const untrustedWriteAccess windows.ACCESS_MASK = 0x00000002 | 0x00000004 | 0x00000010 | 0x00000040 | 0x00010000 | 0x00040000 | 0x00080000 | 0x10000000 | 0x40000000
const windowsCredentialRemediation = "secure the Windows ACL or use the OS keyring backend (credential_ref: keyring:<profile>)"

func currentWindowsUser() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("inspect current Windows user: %w", err)
	}
	return user.User.Sid, nil
}

func openWindowsObject(path string, directory bool) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

func checkCredentialDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve credential directory: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("inspect credential directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("credential directory %s must be a real directory, not a junction or symlink; %s", abs, windowsCredentialRemediation)
	}
	for current := abs; ; current = filepath.Dir(current) {
		f, err := openWindowsObject(current, true)
		if err != nil {
			return fmt.Errorf("open credential directory ancestor for ACL validation: %w; %s", err, windowsCredentialRemediation)
		}
		checkErr := validateWindowsObject(f, false)
		if checkErr == nil && current == abs {
			checkErr = validateWindowsDirectoryOwner(f)
		}
		_ = f.Close()
		if checkErr != nil {
			return fmt.Errorf("credential directory or ancestor ACL is unsafe: %w; %s", checkErr, windowsCredentialRemediation)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return nil
}

func validateWindowsDirectoryOwner(f *os.File) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("inspect credential directory owner: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("read credential directory owner: %w", err)
	}
	user, err := currentWindowsUser()
	if err != nil {
		return fmt.Errorf("cannot establish the current Windows credential owner: %w; %s", err, windowsCredentialRemediation)
	}
	if !windows.EqualSid(owner, user) {
		return fmt.Errorf("credential directory must be owned by the current Windows user")
	}
	return nil
}

func openCredentialFile(path string) (*os.File, error) {
	if err := checkCredentialDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	f, err := openWindowsObject(path, false)
	if err != nil {
		return nil, fmt.Errorf("open credential file for ACL validation: %w; %s", err, windowsCredentialRemediation)
	}
	if err := validateWindowsObject(f, true); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("credential file ACL or type is unsafe: %w; %s", err, windowsCredentialRemediation)
	}
	return f, nil
}

func protectCredentialFile(f *os.File) error {
	user, err := currentWindowsUser()
	if err != nil {
		return fmt.Errorf("cannot establish the current Windows credential owner: %w; %s", err, windowsCredentialRemediation)
	}
	sid := user.String()
	sd, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;;FA;;;" + sid + ")")
	if err != nil {
		return fmt.Errorf("build owner-only credential ACL: %w; %s", err, windowsCredentialRemediation)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read owner-only credential ACL: %w; %s", err, windowsCredentialRemediation)
	}
	if err := windows.SetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("protect credential file ACL: %w; %s", err, windowsCredentialRemediation)
	}
	if err := validateWindowsObject(f, true); err != nil {
		return fmt.Errorf("verify protected credential ACL: %w; %s", err, windowsCredentialRemediation)
	}
	return nil
}

func validateWindowsObject(f *os.File, requireUserOwner bool) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened credential object: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return fmt.Errorf("credential path must be a regular file or real directory")
	}
	var native windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &native); err != nil {
		return fmt.Errorf("inspect credential file attributes: %w", err)
	}
	if native.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("credential path is a reparse point; use a real local file and directory")
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("inspect credential ACL: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("inspect credential owner: %w", err)
	}
	user, err := currentWindowsUser()
	if err != nil {
		return err
	}
	if requireUserOwner && !windows.EqualSid(owner, user) {
		return fmt.Errorf("credential file must be owned by the current Windows user")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("credential ACL is missing or unrestricted")
	}
	control, _, err := sd.Control()
	if err != nil {
		return fmt.Errorf("inspect credential ACL inheritance: %w", err)
	}
	if requireUserOwner && control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("credential file ACL is inherited; protect the file ACL or use the OS keyring backend")
	}
	allowed := map[string]bool{
		user.String():  true,
		"S-1-5-18":     true, // Local System
		"S-1-5-32-544": true, // Built-in Administrators
	}
	seenUser := false
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil || ace == nil {
			return fmt.Errorf("credential ACL contains an unsupported entry")
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("credential ACL contains a deny, partial, or nonstandard access entry")
		}
		if requireUserOwner && ace.Header.AceFlags != 0 {
			return fmt.Errorf("credential file ACL contains inherited access")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		principal := sid.String()
		if requireUserOwner {
			fullAccess := ace.Mask == fileAllAccess || ace.Mask == windows.GENERIC_ALL
			if !fullAccess || !allowed[principal] {
				return fmt.Errorf("credential file ACL grants nonstandard access to Windows principal %s", principal)
			}
		} else if !allowed[principal] && ace.Mask&untrustedWriteAccess != 0 && !(principal == "S-1-3-0" && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0) {
			return fmt.Errorf("credential directory ACL grants write access to an untrusted Windows principal %s", principal)
		}
		if principal == user.String() {
			seenUser = true
		}
	}
	if requireUserOwner && !seenUser {
		return fmt.Errorf("credential ACL does not grant the current Windows user access")
	}
	return nil
}

func checkCredentialFile(path string) error {
	f, err := openCredentialFile(path)
	if err != nil {
		return err
	}
	return f.Close()
}
