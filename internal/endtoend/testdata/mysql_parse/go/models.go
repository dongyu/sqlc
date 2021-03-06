// Code generated by sqlc. DO NOT EDIT.

package querytest

import (
	"database/sql"
	"fmt"
)

type JobStatusType string

const (
	APPLIED  JobStatusType = "APPLIED"
	PENDING  JobStatusType = "PENDING"
	ACCEPTED JobStatusType = "ACCEPTED"
	REJECTED JobStatusType = "REJECTED"
)

func (e *JobStatusType) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = JobStatusType(s)
	case string:
		*e = JobStatusType(s)
	default:
		return fmt.Errorf("unsupported scan type for JobStatusType: %T", src)
	}
	return nil
}

type Order struct {
	ID     int     `json:"id"`
	Price  float64 `json:"price"`
	UserID int     `json:"user_id"`
}

type User struct {
	ID        int            `json:"id"`
	FirstName string         `json:"first_name"`
	LastName  sql.NullString `json:"last_name"`
	Age       int            `json:"age"`
	JobStatus JobStatusType  `json:"job_status"`
}
