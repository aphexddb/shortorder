//go:build windows

package printer

import (
	"fmt"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var vidPidRe = regexp.MustCompile(`(?i)vid_([0-9a-f]{4})&pid_([0-9a-f]{4})`)

// detect enumerates present USB printer-class device interfaces and returns
// those whose USB VID/PID or friendly name matches the supported-model allowlist.
func detect() ([]Info, error) {
	hDevInfo, _, _ := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&guidUSBPrint)),
		0, 0,
		uintptr(digcfPresent|digcfDeviceInterface),
	)
	if hDevInfo == invalidHandleValue || hDevInfo == 0 {
		return nil, fmt.Errorf("SetupDiGetClassDevs failed")
	}
	defer procSetupDiDestroyDeviceInfoList.Call(hDevInfo)

	var out []Info
	for index := 0; ; index++ {
		var ifaceData spDeviceInterfaceData
		ifaceData.cbSize = uint32(unsafe.Sizeof(ifaceData))

		r1, _, _ := procSetupDiEnumDeviceInterfaces.Call(
			hDevInfo, 0,
			uintptr(unsafe.Pointer(&guidUSBPrint)),
			uintptr(index),
			uintptr(unsafe.Pointer(&ifaceData)),
		)
		if r1 == 0 {
			break // no more interfaces
		}

		path, devInfo, ok := interfaceDetail(hDevInfo, &ifaceData)
		if !ok {
			continue
		}

		vid, pid, hasUSB := parseVIDPID(path)
		friendly := registryProperty(hDevInfo, &devInfo, spdrpFriendlyName)
		if friendly == "" {
			friendly = registryProperty(hDevInfo, &devInfo, spdrpDeviceDesc)
		}

		var m *Model
		if hasUSB {
			m = matchByUSB(vid, pid)
		}
		if m == nil {
			m = matchByName(friendly, "")
		}
		if m == nil {
			continue
		}

		usb := ""
		if hasUSB {
			usb = fmt.Sprintf("VID_%04X PID_%04X", vid, pid)
		}
		if friendly == "" {
			friendly = m.Name
		}
		out = append(out, Info{
			Name:  friendly,
			Model: m.Name,
			USB:   usb,
			Path:  path,
		})
	}
	return out, nil
}

// interfaceDetail resolves the device path for an interface and fills a
// SP_DEVINFO_DATA usable for registry-property lookups.
func interfaceDetail(hDevInfo uintptr, iface *spDeviceInterfaceData) (string, spDevInfoData, bool) {
	// First call: required buffer size.
	var required uint32
	procSetupDiGetDeviceInterfaceDetailW.Call(
		hDevInfo,
		uintptr(unsafe.Pointer(iface)),
		0, 0,
		uintptr(unsafe.Pointer(&required)),
		0,
	)
	if required == 0 {
		return "", spDevInfoData{}, false
	}

	buf := make([]byte, required)
	// SP_DEVICE_INTERFACE_DETAIL_DATA_W.cbSize must be 8 on 64-bit (fixed-part
	// size), regardless of the variable-length DevicePath that follows at +4.
	*(*uint32)(unsafe.Pointer(&buf[0])) = 8

	var devInfo spDevInfoData
	devInfo.cbSize = uint32(unsafe.Sizeof(devInfo))

	r1, _, _ := procSetupDiGetDeviceInterfaceDetailW.Call(
		hDevInfo,
		uintptr(unsafe.Pointer(iface)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(required),
		uintptr(unsafe.Pointer(&required)),
		uintptr(unsafe.Pointer(&devInfo)),
	)
	if r1 == 0 {
		return "", spDevInfoData{}, false
	}

	// DevicePath is a NUL-terminated UTF-16 string starting at offset 4.
	pathPtr := (*uint16)(unsafe.Pointer(&buf[4]))
	path := windows.UTF16PtrToString(pathPtr)
	return path, devInfo, true
}

// registryProperty reads a SPDRP_* string property for a device, or "" if absent.
func registryProperty(hDevInfo uintptr, devInfo *spDevInfoData, prop uint32) string {
	var required uint32
	procSetupDiGetDeviceRegistryPropertyW.Call(
		hDevInfo,
		uintptr(unsafe.Pointer(devInfo)),
		uintptr(prop),
		0, 0, 0,
		uintptr(unsafe.Pointer(&required)),
	)
	if required == 0 {
		return ""
	}
	buf := make([]byte, required)
	r1, _, _ := procSetupDiGetDeviceRegistryPropertyW.Call(
		hDevInfo,
		uintptr(unsafe.Pointer(devInfo)),
		uintptr(prop),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(required),
		uintptr(unsafe.Pointer(&required)),
	)
	if r1 == 0 {
		return ""
	}
	return strings.TrimRight(windows.UTF16PtrToString((*uint16)(unsafe.Pointer(&buf[0]))), "\x00")
}

// parseVIDPID extracts the USB vendor/product IDs from a device interface path.
func parseVIDPID(path string) (vid, pid uint16, ok bool) {
	m := vidPidRe.FindStringSubmatch(path)
	if m == nil {
		return 0, 0, false
	}
	return hex4(m[1]), hex4(m[2]), true
}
