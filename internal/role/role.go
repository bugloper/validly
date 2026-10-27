// Package role recognises role-based local parts such as "support" or
// "noreply". A role address usually reaches a team or nobody rather than one
// person. The result is informational: role addresses are valid and Validly
// never rejects them on that basis.
package role

import "strings"

var roles = map[string]struct{}{}

func init() {
	for _, r := range strings.Fields(`
		abuse accounting accounts admin administrator billing business careers
		contact customercare customerservice customersupport devnull dev
		do-not-reply donotreply enquiries feedback finance help helpdesk hostmaster
		hr info inquiries it jobs legal mail mailer-daemon marketing media no-reply
		noc noreply null office orders postmaster press privacy recruitment root
		sales security service services spam staff support sysadmin team tech
		undisclosed-recipients unsubscribe usenet uucp webmaster www
	`) {
		roles[r] = struct{}{}
	}
}

// IsRole reports whether local is a common role local part. Matching ignores
// case and any "+tag" suffix.
func IsRole(local string) bool {
	local = strings.ToLower(local)
	if i := strings.IndexByte(local, '+'); i >= 0 {
		local = local[:i]
	}
	_, ok := roles[local]
	return ok
}
