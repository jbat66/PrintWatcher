package db

import (
	"github.com/jbat66/PrintWatcher/models"
	"testing"
)

func TestPrinterNotificationSoundsPersistIndependently(t *testing.T) {
	database, err := Open(t.TempDir() + "/sounds.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	a := models.PrinterConfig{Name: "A", NotificationSound: "Cuckoo.mp3"}
	b := models.PrinterConfig{Name: "B", NotificationSound: "RedAlert.mp3"}
	for _, p := range []*models.PrinterConfig{&a, &b} {
		if err := database.CreatePrinter(p); err != nil {
			t.Fatal(err)
		}
	}
	a.NotificationSound = "ClockChimes.mp3"
	if err := database.UpdatePrinter(&a); err != nil {
		t.Fatal(err)
	}
	for _, want := range []models.PrinterConfig{a, b} {
		got, err := database.GetPrinter(want.ID)
		if err != nil || got.NotificationSound != want.NotificationSound {
			t.Fatalf("sound not saved: %+v, %v", got, err)
		}
	}
	list, err := database.ListPrinters()
	if err != nil || len(list) != 2 || list[0].NotificationSound != a.NotificationSound || list[1].NotificationSound != b.NotificationSound {
		t.Fatalf("unexpected sound list: %+v, %v", list, err)
	}
}
