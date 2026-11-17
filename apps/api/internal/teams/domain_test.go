package teams

import (
	"testing"

	"github.com/google/uuid"
)

func TestPermissionsFollowTheTeamMatrix(t *testing.T) {
	teamID := uuid.New()
	for _, tc := range []struct {
		role     string
		manage   bool
		canLeave bool
	}{
		{RoleOwner, true, true},
		{RoleAdmin, true, true},
		{RoleMember, false, true},
	} {
		p := permissionsFor(teamID, tc.role)
		if p.TeamID != teamID || p.Role != tc.role {
			t.Errorf("%s: identity fields wrong: %+v", tc.role, p)
		}
		if p.CanInvite != tc.manage || p.CanManageInvitations != tc.manage ||
			p.CanRemoveMembers != tc.manage || p.CanChangeRoles != tc.manage {
			t.Errorf("%s: management capabilities = %+v, want manage=%v", tc.role, p, tc.manage)
		}
		if p.CanLeave != tc.canLeave {
			t.Errorf("%s: can_leave = %v", tc.role, p.CanLeave)
		}
	}
}

func TestRoleValidation(t *testing.T) {
	for _, role := range []string{RoleOwner, RoleAdmin, RoleMember} {
		if !validTeamRole(role) {
			t.Errorf("team role %q must be valid", role)
		}
	}
	for _, role := range []string{"", "root", "platform_admin", "superuser"} {
		if validTeamRole(role) {
			t.Errorf("team role %q must be rejected", role)
		}
	}
	if validInvitationRole(RoleOwner) {
		t.Error("invitations must never offer the owner role")
	}
	for _, role := range []string{RoleAdmin, RoleMember} {
		if !validInvitationRole(role) {
			t.Errorf("invitation role %q must be valid", role)
		}
	}
}
