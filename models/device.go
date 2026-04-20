package models

import "time"

// Device represents an equipment/device available for rent
type Device struct {
	DeviceNo     int       `json:"deviceNo"`
	DeviceName   string    `json:"deviceName"`
	Description  string    `json:"description"`
	RentPrice    float64   `json:"rentPrice"`
	TypeNo       *int      `json:"typeNo,omitempty"`
	DeviceTypeNo *int      `json:"deviceTypeNo,omitempty"`
	Rating       float64   `json:"rating"`
	UserId       int       `json:"userId"`
	Status       string    `json:"status"`
	Condition    string    `json:"condition,omitempty"`
	ImageUrl     string    `json:"imageUrl,omitempty"`
	CPU          *string   `json:"cpu,omitempty"`
	RAM          *string   `json:"ram,omitempty"`
	Storage      *string   `json:"storage,omitempty"`
	GPU          *string   `json:"gpu,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// DeviceType represents the type of device (Notebook, MacBook, Other)
type DeviceType struct {
	DeviceTypeNo   int    `json:"deviceTypeNo"`
	DeviceTypeName string `json:"deviceTypeName"`
}
