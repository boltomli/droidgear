// Native notifications via the Wails v3 notifications service
// (pkg/services/notifications): Windows toast, macOS UNUserNotificationCenter
// and the Linux freedesktop daemon behind one API. This replaces the
// earlier PowerShell toast with an unregistered AUMID, which could fail
// silently and offered no permission handling (guidelines §3.5 / P1-4).
// main.go installs the shared service instance via SetNotificationService;
// on platforms that require it (macOS), permission is requested lazily on
// first send.
package app

import (
	"fmt"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// notificationService is the shared Wails notifications service.
var notificationService *notifications.NotificationService

// SetNotificationService injects the notifications service registered by
// main.go (Wails services are plain structs; the notification service is
// created once so its scheduled toasts and categories stay consistent).
func SetNotificationService(service *notifications.NotificationService) {
	notificationService = service
}

// SendNativeNotification shows a system notification (mirrors
// notifications::send_native_notification).
func (a *App) SendNativeNotification(title string, body *string) error {
	if notificationService == nil {
		return errString("Native notifications are not available in this build")
	}
	if runtime.GOOS != "windows" {
		// macOS requires user permission; the prompt is shown at most once
		// per install. A failed check is ignored — sending then surfaces
		// the concrete platform error instead.
		if granted, err := notificationService.CheckNotificationAuthorization(); err == nil && !granted {
			if _, err := notificationService.RequestNotificationAuthorization(); err != nil {
				return fmt.Errorf("Failed to request notification permission: %w", err)
			}
		}
	}
	options := notifications.NotificationOptions{Title: title}
	if body != nil {
		options.Body = *body
	}
	if err := notificationService.SendNotification(options); err != nil {
		return fmt.Errorf("Failed to send notification: %w", err)
	}
	return nil
}
