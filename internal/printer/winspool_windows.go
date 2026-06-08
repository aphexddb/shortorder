//go:build windows

package printer

import (
	"golang.org/x/sys/windows"
)

// Windows USB printer-class device bindings.
//
// Budget receipt printers like the Volcora v-WRP2-A1W bind to the in-box
// usbprint.sys driver and expose a USB printer device interface
// (GUID_DEVINTERFACE_USBPRINT) but frequently have NO installed spooler queue.
// We therefore find them via SetupAPI device-interface enumeration and write raw
// ESC/POS bytes straight to the device with CreateFile/WriteFile — no print
// queue, driver install, or admin rights required.
var (
	modSetupapi = windows.NewLazySystemDLL("setupapi.dll")

	procSetupDiGetClassDevsW              = modSetupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces       = modSetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW  = modSetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiGetDeviceRegistryPropertyW = modSetupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procSetupDiDestroyDeviceInfoList      = modSetupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

// GUID_DEVINTERFACE_USBPRINT {28d78fad-5a12-11d1-ae5b-0000f803a8c2}
var guidUSBPrint = windows.GUID{
	Data1: 0x28d78fad,
	Data2: 0x5a12,
	Data3: 0x11d1,
	Data4: [8]byte{0xae, 0x5b, 0x00, 0x00, 0xf8, 0x03, 0xa8, 0xc2},
}

// SetupDiGetClassDevs flags.
const (
	digcfPresent         = 0x00000002
	digcfDeviceInterface = 0x00000010
)

// SetupDiGetDeviceRegistryProperty property codes.
const (
	spdrpDeviceDesc   = 0x00000000
	spdrpFriendlyName = 0x0000000C
)

// invalidHandleValue is what SetupDiGetClassDevs returns on failure.
const invalidHandleValue = ^uintptr(0)

// spDeviceInterfaceData mirrors SP_DEVICE_INTERFACE_DATA.
type spDeviceInterfaceData struct {
	cbSize             uint32
	interfaceClassGUID windows.GUID
	flags              uint32
	reserved           uintptr
}

// spDevInfoData mirrors SP_DEVINFO_DATA.
type spDevInfoData struct {
	cbSize    uint32
	classGUID windows.GUID
	devInst   uint32
	reserved  uintptr
}
