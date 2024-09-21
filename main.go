package main

import (
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/esiqveland/notify"
	"github.com/godbus/dbus/v5"
)

const (
	BatteryStatusIndicator = "/sys/class/power_supply/BAT0/status"
	EnergyWhenFull         = "/sys/class/power_supply/BAT0/energy_full"
	EnergyNow              = "/sys/class/power_supply/BAT0/energy_now"
)

// the threshold at which we should start alerting about charging
const CriticalEnergyLevel = 24

type chargingState = int

const (
	Unknown chargingState = iota + 1
	Charging
	Discharging
	NotCharging
	Full
)

// battery is data representing the battery inside a laptop :)
type battery struct {
	// Whether or not the battery is being charged, however we support
	// all of the "charging" states in linux/power_suppy.h
	chargeState chargingState

	// current battery percentage
	energyLevel float64

	// last notification sent
	lastMsg time.Time

	// how we send notifications
	notifier notify.Notifier
}

func newBatStat() *battery {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		panic(err)
	}

	err = conn.Auth(nil)
	if err != nil {
		panic(err)
	}

	err = conn.Hello()
	if err != nil {
		panic(err)
	}

	notifier, err := notify.New(conn)
	if err != nil {
		panic(err)
	}

	return &battery{
		energyLevel: 99.99, chargeState: NotCharging, notifier: notifier}
}

func main() {
	bs := newBatStat()
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case _ = <-ticker.C:
			bs.writeBatPercentage()
			bs.writeChargingState()
			bs.sendChargeMsg()
		}
	}
}

func (bat *battery) writeChargingState() {
	st, err := os.ReadFile(BatteryStatusIndicator)
	if err != nil {
		panic("bat: unable to read BAT0 status")
	}
	status := string(st)

	if strings.Trim(status, "\n") == "Charging" {
		if bat.chargeState != Charging {
			bat.sendChargingMsg()
		}
		bat.chargeState = Charging
		return
	}

	bat.chargeState = Discharging
}

func (bat *battery) writeBatPercentage() {
	ef, err := os.ReadFile(EnergyWhenFull)
	if err != nil {
		panic("bat: unable to read BAT0 status")
	}
	en, err := os.ReadFile(EnergyNow)
	if err != nil {
		panic("bat: unable to read BAT0 status")
	}

	// NOTE(nate): not sure that I have to have these as uint32_t
	// just making an assumption and frankly this works so :)
	energy_full, err := strconv.ParseUint(strings.Trim(string(ef), "\n"), 10, 32)
	if err != nil {
		panic("bat: unable to parse energy_full")
	}
	energy_now, err := strconv.ParseUint(strings.Trim(string(en), "\n"), 10, 32)
	if err != nil {
		panic("bat: unable to parse energy_now")
	}

	// NOTE(nate): this seems silly
	bat.energyLevel =
		math.Floor(float64(100.00 * (float32(energy_now) / float32(energy_full))))
}

func (bat battery) sendChargingMsg() {

	// Create a Notification to send
	n := notify.Notification{
		AppName:       "Battery-Notify",
		ReplacesID:    uint32(0),
		Summary:       "Battery Status",
		Body:          "Charging",
		ExpireTimeout: time.Second * 5,
	}
	_, err := bat.notifier.SendNotification(n)
	if err != nil {
		log.Printf("error sending notification: %v", err)
	}

}

func (bat *battery) sendChargeMsg() {
	if time.Since(bat.lastMsg) <= 10*time.Second || bat.chargeState == Charging {
		return
	}

	if bat.energyLevel > CriticalEnergyLevel {
		return
	}
	// Create a Notification to send
	n := notify.Notification{
		AppName:       "Battery-Notify",
		ReplacesID:    uint32(0),
		Summary:       "Battery leveling  warning",
		Body:          "Please charge your laptop!",
		ExpireTimeout: time.Second * 5,
	}
	n.SetUrgency(notify.UrgencyCritical)
	n.AddHint(notify.HintUrgency(notify.UrgencyCritical))

	_, err := bat.notifier.SendNotification(n)
	bat.lastMsg = time.Now()

	if err != nil {
		log.Printf("error sending notification: %v", err)
	}
}
