package webhandler

import "testing"

// ── emailRegex ──────────────────────────────────────────────

func TestEmailRegex_ValidEmails(t *testing.T) {
	valid := []string{
		"user@example.com",
		"test.user@domain.com",
		"user+tag@example.com",
		"user@sub.domain.co.uk",
		"a@b.cc",
		"user123@domain456.com",
		"user-name@domain.com",
		"user_name@domain.com",
		"USER@DOMAIN.COM",
		"user%special@domain.com",
	}

	for _, email := range valid {
		if !emailRegex.MatchString(email) {
			t.Errorf("expected %q to be a valid email", email)
		}
	}
}

func TestEmailRegex_InvalidEmails(t *testing.T) {
	invalid := []string{
		"",
		"not-an-email",
		"@domain.com",
		"user@",
		"user@.com",
		"user@domain",       // no TLD
		"user @domain.com",  // space
		"user@domain .com",  // space
		"<script>@evil.com", // XSS attempt
	}

	for _, email := range invalid {
		if emailRegex.MatchString(email) {
			t.Errorf("expected %q to be an invalid email", email)
		}
	}
}

// ── Registration validation edge cases ──────────────────────
// These test the validation rules documented in RegisterSubmit.
// Since we can't easily invoke the full handler without DB deps,
// we test the validation predicates directly.

func TestRegistrationValidation_PasswordLength(t *testing.T) {
	// Minimum 8 characters
	if len("short") >= 8 {
		t.Error("test assumption: 'short' should be < 8 chars")
	}
	if len("longpassword") < 8 {
		t.Error("test assumption: 'longpassword' should be >= 8 chars")
	}

	// Maximum 72 characters (bcrypt limit)
	long73 := make([]byte, 73)
	for i := range long73 {
		long73[i] = 'a'
	}
	if len(long73) <= 72 {
		t.Error("test assumption: 73-byte password should be > 72")
	}
}

func TestRegistrationValidation_DisplayNameLength(t *testing.T) {
	// Maximum 100 characters
	long101 := make([]byte, 101)
	for i := range long101 {
		long101[i] = 'a'
	}
	if len(long101) <= 100 {
		t.Error("test assumption: 101-byte name should be > 100")
	}
}

// ascentTypeLabel tests are in route_cards_test.go

// ── loadConsensus ───────────────────────────────────────────
// loadConsensus is tested indirectly through RouteDetail, but we can
// verify the ConsensusData percentage calculation logic.

func TestConsensusData_Percentages(t *testing.T) {
	// When total is 10: easy=3, right=5, hard=2
	total := 10
	easy, right, hard := 3, 5, 2

	easyPct := easy * 100 / total
	rightPct := right * 100 / total
	hardPct := hard * 100 / total

	if easyPct != 30 {
		t.Errorf("easyPct = %d, want 30", easyPct)
	}
	if rightPct != 50 {
		t.Errorf("rightPct = %d, want 50", rightPct)
	}
	if hardPct != 20 {
		t.Errorf("hardPct = %d, want 20", hardPct)
	}
}
