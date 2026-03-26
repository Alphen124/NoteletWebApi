package requests

// CreateDeviceRequest represents the request body for creating a new device
type CreateDeviceRequest struct {
	Name        string  `json:"name" validate:"required"`
	Type        string  `json:"type" validate:"required,oneof=Notebook MacBook Other"`
	Price       float64 `json:"price" validate:"required,min=0"`
	Description string  `json:"description"`
	Condition   string  `json:"condition" validate:"omitempty,oneof=new like-new good fair poor"`
	ImageUrl    string  `json:"imageUrl"`
}

// UpdateDeviceRequest represents the request body for updating a device
type UpdateDeviceRequest struct {
	Name        *string  `json:"name,omitempty"`
	Type        *string  `json:"type,omitempty" validate:"omitempty,oneof=Notebook MacBook Other"`
	Price       *float64 `json:"price,omitempty" validate:"omitempty,min=0"`
	Description *string  `json:"description,omitempty"`
	Status      *string  `json:"status,omitempty" validate:"omitempty,oneof=available rented"`
	Condition   *string  `json:"condition,omitempty" validate:"omitempty,oneof=new like-new good fair poor"`
	ImageUrl    *string  `json:"imageUrl,omitempty"`
}

// UpdateDeviceStatusRequest represents the request body for updating device status
type UpdateDeviceStatusRequest struct {
	Status string `json:"status" validate:"required"`
}
