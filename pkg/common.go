package pkg

import "regexp"

func ValidatePassword(password string) bool {
	// Check if the password contains at least two of the following character types
	uppercaseRegex := regexp.MustCompile(`[A-Z]`)
	lowercaseRegex := regexp.MustCompile(`[a-z]`)
	numberRegex := regexp.MustCompile(`[0-9]`)
	symbolRegex := regexp.MustCompile(`[!@#$%^&*()_+-=\[\]\\{}|;':",./<>?~]`)
	sysbolRegexk := regexp.MustCompile("`")
	sum := 0
	if uppercaseRegex.MatchString(password) {
		sum += 1
	}
	if lowercaseRegex.MatchString(password) {
		sum += 1
	}
	if numberRegex.MatchString(password) {
		sum += 1
	}
	if symbolRegex.MatchString(password) || sysbolRegexk.MatchString(password) {
		sum += 1
	}

	return sum >= 2
}
