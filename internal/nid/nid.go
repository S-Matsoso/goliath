package nid

import (
	"fmt"
	"time"
)

// Validate checks the 13-digit format, encoded birth date, and Luhn digit.
func Validate(value string, dateOfBirth time.Time) error {
	if err := ValidateFormat(value); err != nil {
		return err
	}
	if value[:6] != dateOfBirth.Format("060102") {
		return fmt.Errorf("national ID birth date does not match date_of_birth")
	}
	return nil
}

// ValidateFormat checks the 13 ASCII digits and Luhn check digit.
func ValidateFormat(value string) error {
	if len(value) != 13 {
		return fmt.Errorf("national ID must contain 13 digits")
	}
	for i := range value {
		if value[i] < '0' || value[i] > '9' {
			return fmt.Errorf("national ID must contain only ASCII digits")
		}
	}

	sum := 0
	double := true
	for i := len(value) - 2; i >= 0; i-- {
		digit := int(value[i] - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	checkDigit := (10 - sum%10) % 10
	if int(value[len(value)-1]-'0') != checkDigit {
		return fmt.Errorf("national ID check digit is invalid")
	}
	return nil
}
