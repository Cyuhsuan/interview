// Package slotdto holds the appointment-slot DTO and service-code lookup
// shared by the booking and conversation handlers, so both response shapes
// stay byte-for-byte identical without copy-pasting the mapping logic.
// (booking 與 conversation handler 共用的 slot DTO 與 serviceCode 查詢，
// 避免兩處各自維護一份容易失去同步的映射邏輯。)
package slotdto

import (
	"context"
	"time"

	"backend/internal/model"
	catalogsvc "backend/internal/service/catalog"
)

// Slot is the wire representation of an appointment slot.
type Slot struct {
	ProfessionalID string `json:"professionalId"`
	Start          string `json:"start"`
	End            string `json:"end"`
	TimeZone       string `json:"timeZone"`
}

// FromSession builds a *Slot from a booking session's selected-slot fields,
// returning nil when no slot has been selected yet.
func FromSession(session *model.BookingSession) *Slot {
	if session.ProfessionalID == nil || session.SlotStartAt == nil || session.SlotEndAt == nil {
		return nil
	}
	timeZone := ""
	if session.SlotTimeZone != nil {
		timeZone = *session.SlotTimeZone
	}
	return &Slot{
		ProfessionalID: *session.ProfessionalID,
		Start:          session.SlotStartAt.Format(time.RFC3339),
		End:            session.SlotEndAt.Format(time.RFC3339),
		TimeZone:       timeZone,
	}
}

// ServiceCodeForID looks up the catalog service code for a service ID,
// returning (nil, nil) when id is nil or no active service matches it.
func ServiceCodeForID(ctx context.Context, catalog *catalogsvc.Service, id *string) (*string, error) {
	if id == nil {
		return nil, nil
	}
	services, err := catalog.ListActiveServices(ctx)
	if err != nil {
		return nil, err
	}
	for _, svc := range services {
		if svc.ID == *id {
			code := svc.Code
			return &code, nil
		}
	}
	return nil, nil
}
