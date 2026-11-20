//go:build integration

package contributions_test

import (
	"testing"

	"github.com/google/uuid"
)

func TestPhase06M2ContributionReusedAcrossRealFundingRounds(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	caseID := f.create()
	f.prepare(caseID)
	f.confirm(caseID)

	var caseVersion, confirmation uuid.UUID
	var digest string
	must(t, f.db.Pool.QueryRow(f.ctx,
		"SELECT c.effective_confirmed_version_id,f.id,f.version_digest FROM contribution_cases c JOIN contribution_confirmations f ON f.case_version_id=c.effective_confirmed_version_id WHERE c.id=$1",
		caseID).Scan(&caseVersion, &confirmation, &digest))

	lead := f.actors["lead"].UserID
	worker := f.actors["worker"].UserID
	roundA, roundB := uuid.New(), uuid.New()
	verA, verB := uuid.New(), uuid.New()
	sourceA, sourceB := uuid.New(), uuid.New()

	insertRound := "INSERT INTO bonus_rounds(id,team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,$3,$4,'Prize','competition',$5)"
	_, err := f.db.Pool.Exec(f.ctx, insertRound, roundA, f.team, f.project.ID, "school-award-2026", lead)
	must(t, err)
	_, err = f.db.Pool.Exec(f.ctx, insertRound, roundB, f.team, f.project.ID, "college-award-2026", lead)
	must(t, err)

	insertVersion := "INSERT INTO bonus_round_versions(id,team_id,project_id,round_id,version,total_budget_minor,plan_unallocated_minor,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,$4,1,100,0,$5,'M2 draft only',$6)"
	_, err = f.db.Pool.Exec(f.ctx, insertVersion, verA, f.team, f.project.ID, roundA, lead, uuid.New())
	must(t, err)
	_, err = f.db.Pool.Exec(f.ctx, insertVersion, verB, f.team, f.project.ID, roundB, lead, uuid.New())
	must(t, err)

	insertSource := "INSERT INTO bonus_round_sources(id,team_id,project_id,round_id,version_id,task_id,case_id,case_version_id,confirmation_id,confirmation_digest,coverage_decision,source_budget_minor,source_unallocated_minor,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'included',100,$11,'confirmed evidence')"
	_, err = f.db.Pool.Exec(f.ctx, insertSource, uuid.New(), f.team, f.project.ID, roundB, verB, f.task.ID, caseID, caseVersion, confirmation, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", int64(0))
	if err == nil {
		t.Fatal("tampered confirmation digest was accepted")
	}
	_, err = f.db.Pool.Exec(f.ctx, insertSource, sourceA, f.team, f.project.ID, roundA, verA, f.task.ID, caseID, caseVersion, confirmation, digest, int64(0))
	must(t, err)
	_, err = f.db.Pool.Exec(f.ctx, insertSource, sourceB, f.team, f.project.ID, roundB, verB, f.task.ID, caseID, caseVersion, confirmation, digest, int64(100))
	must(t, err)
	_, err = f.db.Pool.Exec(f.ctx, insertSource, uuid.New(), f.team, f.project.ID, roundA, verA, f.task.ID, caseID, caseVersion, confirmation, digest, int64(0))
	if err == nil {
		t.Fatal("same confirmation twice within a bonus version was accepted")
	}

	var epoch int
	must(t, f.db.Pool.QueryRow(f.ctx,
		"SELECT authorization_version FROM project_members WHERE team_id=$1 AND project_id=$2 AND user_id=$3",
		f.team, f.project.ID, worker).Scan(&epoch))
	eligA, eligB := uuid.New(), uuid.New()
	insertEligibility := "INSERT INTO bonus_eligibility_decisions(id,team_id,project_id,round_id,version_id,user_id,decision,member_authorization_version,decided_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'draft eligibility')"
	_, err = f.db.Pool.Exec(f.ctx, insertEligibility, eligA, f.team, f.project.ID, roundA, verA, worker, "eligible", epoch, lead)
	must(t, err)
	_, err = f.db.Pool.Exec(f.ctx, insertEligibility, eligB, f.team, f.project.ID, roundB, verB, worker, "ineligible", epoch, lead)
	must(t, err)

	insertLine := "INSERT INTO bonus_allocation_lines(team_id,project_id,round_id,version_id,source_id,case_version_id,user_id,eligibility_decision_id,share_bps,raw_rounded_minor,suggested_minor,final_minor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)"
	_, err = f.db.Pool.Exec(f.ctx, insertLine, f.team, f.project.ID, roundA, verA, sourceA, caseVersion, worker, eligA, 5000, int64(100), int64(100), int64(100))
	if err == nil {
		t.Fatal("a share different from the confirmed evidence was accepted")
	}
	_, err = f.db.Pool.Exec(f.ctx, insertLine, f.team, f.project.ID, roundA, verA, sourceA, caseVersion, worker, eligA, 10000, int64(100), int64(100), int64(100))
	must(t, err)
	_, err = f.db.Pool.Exec(f.ctx, insertLine, f.team, f.project.ID, roundB, verB, sourceB, caseVersion, worker, eligB, 10000, int64(100), int64(100), int64(100))
	if err == nil {
		t.Fatal("ineligible participant received money")
	}
	_, err = f.db.Pool.Exec(f.ctx, insertLine, f.team, f.project.ID, roundB, verB, sourceB, caseVersion, worker, eligB, 10000, int64(100), int64(0), int64(0))
	must(t, err)

	var totalA, totalB int64
	must(t, f.db.Pool.QueryRow(f.ctx,
		"SELECT total_minor FROM bonus_member_totals WHERE round_id=$1 AND version_id=$2 AND user_id=$3",
		roundA, verA, worker).Scan(&totalA))
	must(t, f.db.Pool.QueryRow(f.ctx,
		"SELECT total_minor FROM bonus_member_totals WHERE round_id=$1 AND version_id=$2 AND user_id=$3",
		roundB, verB, worker).Scan(&totalB))
	if totalA != 100 || totalB != 0 {
		t.Fatalf("different funding rounds share evidence but not money: %d / %d", totalA, totalB)
	}
	_, err = f.db.Pool.Exec(f.ctx,
		"UPDATE bonus_allocation_lines SET final_minor=0 WHERE round_id=$1 AND source_id=$2 AND user_id=$3",
		roundA, sourceA, worker)
	if err == nil {
		t.Fatal("M2 must not permit unaudited draft line edits")
	}
}
