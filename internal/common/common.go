package common

import "regexp"

func ValidatePassword(password string) bool {
	// Check if the password contains at least two of the following character types
	if len([]rune(password)) < 6 || len([]rune(password)) > 12 {
		return false
	}
	uppercaseRegex := regexp.MustCompile(`[A-Z]`)
	lowercaseRegex := regexp.MustCompile(`[a-z]`)
	numberRegex := regexp.MustCompile(`[0-9]`)
	symbolRegex := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]\\{}|;':",./<>?~]`)
	sysbolRegexk := regexp.MustCompile("`")
	sum := 0
	if !numberRegex.MatchString(password) {
		return false
	}
	if uppercaseRegex.MatchString(password) {
		sum += 1
	}
	if lowercaseRegex.MatchString(password) {
		sum += 1
	}
	if symbolRegex.MatchString(password) || sysbolRegexk.MatchString(password) {
		sum += 1
	}

	return sum >= 1
}
