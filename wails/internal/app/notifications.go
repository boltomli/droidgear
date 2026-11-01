// Native notifications. Wails v3 has no cross-platform notification API, so
// Windows uses a PowerShell toast (hidden console); other platforms return an
// error which the frontend's notify() falls back to a toast for.
package app

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// SendNativeNotification shows a system notification (mirrors
// notifications::send_native_notification).
func (a *App) SendNativeNotification(title string, body *string) error {
	if runtime.GOOS != "windows" {
		return errString("Native notifications are only supported on Windows in this build")
	}

	var sb strings.Builder
	// Escape single quotes for the PowerShell single-quoted literal.
	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	sb.WriteString("[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null;")
	sb.WriteString("[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom, ContentType = WindowsRuntime] | Out-Null;")
	sb.WriteString(fmt.Sprintf("$t = '<toast><visual><binding template=\"ToastGeneric\"><text>%s</text>%s</binding></visual></toast>';",
		esc(title),
		func() string {
			if body == nil || *body == "" {
				return ""
			}
			return fmt.Sprintf("<text>%s</text>", esc(*body))
		}()))
	sb.WriteString("$x = New-Object Windows.Data.Xml.Dom.XmlDocument; $x.LoadXml($t);")
	sb.WriteString("[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('DroidGear').Show([Windows.UI.Notifications.ToastNotification]::new($x))")

	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", sb.String()) // #nosec G204 -- static command, user data only interpolated as escaped literals
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Failed to send notification: %w", err)
	}
	// Fire and forget — the toast host outlives this call.
	go func() { _ = cmd.Wait() }()
	return nil
}
