package nid

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

func Generate(dateOfBirth time.Time, sex, citizenship string) (string, error) {
	citizenshipDigit, err := citizenshipCode(citizenship)
	if err != nil {
		return "", err
	}
	sequence, err := sexBandedSequence(sex)
	if err != nil {
		return "", err
	}
	body := dateOfBirth.Format("060102") + fmt.Sprintf("%04d", sequence) + citizenshipDigit + "0"
	check, err := checkDigit(body)
	if err != nil {
		return "", err
	}
	value := body + string(check)
	if err := Validate(value, dateOfBirth); err != nil {
		return "", err
	}
	return value, nil
}

func sexBandedSequence(sex string) (int, error) {
	max := big.NewInt(5000)
	value, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0, fmt.Errorf("generate national ID sequence: %w", err)
	}
	sequence := int(value.Int64()) * 2
	switch sex {
	case "M":
		sequence++
	case "F":
	case "X":
		if sequence%2 == 0 {
			sequence++
		}
	default:
		return 0, fmt.Errorf("unsupported sex code")
	}
	return sequence, nil
}

func citizenshipCode(status string) (string, error) {
	switch status {
	case "CITIZEN":
		return "1", nil
	case "PERMANENT_RESIDENT":
		return "2", nil
	case "FOREIGN":
		return "3", nil
	case "STATELESS":
		return "4", nil
	default:
		return "", fmt.Errorf("unsupported citizenship status")
	}
}

func checkDigit(body string) (byte, error) {
	if len(body) != 12 {
		return 0, fmt.Errorf("national ID body must contain 12 digits")
	}
	sum := 0
	double := true
	for i := len(body) - 1; i >= 0; i-- {
		digit := int(body[i] - '0')
		if digit < 0 || digit > 9 {
			return 0, fmt.Errorf("national ID body must contain digits")
		}
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return byte('0' + (10-sum%10)%10), nil
}
