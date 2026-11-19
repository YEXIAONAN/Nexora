//go:build integration

package contributions_test

import (
 "github.com/google/uuid"
 "testing"
)

func TestPhase06M2SameConfirmedContributionAcrossDistinctFundingRounds(t *testing.T) {
 f := setup(t)
 f.grant("appraiser")
 caseID := f.create()
 f.prepare(caseID)
 f.confirm(caseID)

 var caseVersion, confirmation uuid.UUID
 var digest string
 must(t, f.db.Pool.QueryRow(f.ctx,
  "SELECT c.effective_confirmed_version_id, f.id, f.version_digest FROM contribution_cases c JOIN contribution_confirmations f ON f.case_version_id=c.effective_confirmed_version_id WHERE c.id=$1",
  caseID).Scan(&caseVersion,&confirmation,&digest))

 lead := f.actors["lead"].UserID
 worker := f.actors["worker"].UserID
 roundA, roundB, verA, verB := uuid.New(),uuid.New(),uuid.New(),uuid.New()
 for _, round := range []struct{id uuid.UUID; key string}{
  {roundA,"school-2026-prize"},{roundB,"college-2026-prize"},
 } {
  _, err := f.db.Pool.Exec(f.ctx,
   "INSERT INTO bonus_rounds(id,team_id,project_id,funding_key,funding_label,funding_source,created_by) VALUES($1,$2,$3,$4,$4,'competition awards',$5)",
   round.id,f.team,f.project.ID,round.key,lead)
  must(t,err)
 }
 for _, vr := range []struct{round,version uuid.UUID}{
  {roundA,verA},{roundB,verB},
 } {
  _, err := f.db.Pool.Exec(f.ctx,
   "INSERT INTO bonus_round_versions(id,team_id,project_id,round_id,version,total_budget_minor,plan_unallocated_minor,proposed_by,rationale,client_request_id) VALUES($1,$2,$3,$4,1,100,0,$5,'M2 structural draft',$6)",
   vr.version,f.team,f.project.ID,vr.round,lead,uuid.New())
  must(t,err)
 }
 sourceInsert := "INSERT INTO bonus_round_sources(id,team_id,project_id,round_id,version_id,task_id,case_id,case_version_id,confirmation_id,confirmation_digest,coverage_decision,source_budget_minor,source_unallocated_minor,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'included',100,$11,'explicit draft source')"
 sourceA,sourceB := uuid.New(),uuid.New()
 // The digest must match the *effective* Phase05 confirmation, even when
 // the project stays active and the draft uses the correct confirmation ID.
 _, err := f.db.Pool.Exec(f.ctx,sourceInsert,uuid.New(),f.team,f.project.ID,roundB,verB,f.task.ID,caseID,caseVersion,confirmation,"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",0)
 if err == nil { t.Fatal("tampered contribution digest accepted") }
 _, err = f.db.Pool.Exec(f.ctx,sourceInsert,sourceA,f.team,f.project.ID,roundA,verA,f.task.ID,caseID,caseVersion,confirmation,digest,0)
 must(t,err)
 _, err = f.db.Pool.Exec(f.ctx,sourceInsert,sourceB,f.team,f.project.ID,roundB,verB,f.task.ID,caseID,caseVersion,confirmation,digest,100)
 must(t,err)
 // D1/A1: same evidence is legal across two independent prize rounds.
 // The same evidence cannot be bound twice within one round version.
 _, err = f.db.Pool.Exec(f.ctx,sourceInsert,uuid.New(),f.team,f.project.ID,roundA,verA,f.task.ID,caseID,caseVersion,confirmation,digest,0)
 if err == nil { t.Fatal("duplicate confirmation within a round version accepted") }

 var memberEpoch int
 must(t, f.db.Pool.QueryRow(f.ctx,
  "SELECT authorization_version FROM project_members WHERE team_id=$1 AND project_id=$2 AND user_id=$3",
  f.team,f.project.ID,worker).Scan(&memberEpoch))
 eligA,eligB := uuid.New(),uuid.New()
 for _, e := range []struct{round,version,decisionID uuid.UUID; decision string}{
  {roundA,verA,eligA,"eligible"},{roundB,verB,eligB,"ineligible"},
 } {
  _, err := f.db.Pool.Exec(f.ctx,
   "INSERT INTO bonus_eligibility_decisions(id,team_id,project_id,round_id,version_id,user_id,decision,member_authorization_version,decided_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'explicit draft decision')",
   e.decisionID,f.team,f.project.ID,e.round,e.version,worker,e.decision,memberEpoch,lead)
  must(t,err)
 }
 lineInsert := "INSERT INTO bonus_allocation_lines(team_id,project_id,round_id,version_id,source_id,case_version_id,user_id,eligibility_decision_id,share_bps,raw_rounded_minor,suggested_minor,final_minor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)"
 _, err = f.db.Pool.Exec(f.ctx,lineInsert,f.team,f.project.ID,roundA,verA,sourceA,caseVersion,worker,eligA,5000,100,100,100)
 if err == nil { t.Fatal("incorrect confirmed share_bps accepted") }
 _, err = f.db.Pool.Exec(f.ctx,lineInsert,f.team,f.project.ID,roundA,verA,sourceA,caseVersion,worker,eligA,10000,100,100,100)
 must(t,err)
 _, err = f.db.Pool.Exec(f.ctx,lineInsert,f.team,f.project.ID,roundB,verB,sourceB,caseVersion,worker,eligB,10000,100,100,100)
 if err == nil { t.Fatal("ineligible member received a nonzero suggestion or final amount") }
 _, err = f.db.Pool.Exec(f.ctx,lineInsert,f.team,f.project.ID,roundB,verB,sourceB,caseVersion,worker,eligB,10000,100,0,0)
 must(t,err)
 var sumA,sumB int64
 must(t,f.db.Pool.QueryRow(f.ctx,
  "SELECT total_minor FROM bonus_member_totals WHERE round_id=$1 AND version_id=$2 AND user_id=$3",
  roundA,verA,worker).Scan(&sumA))
 must(t,f.db.Pool.QueryRow(f.ctx,
  "SELECT total_minor FROM bonus_member_totals WHERE round_id=$1 AND version_id=$2 AND user_id=$3",
  roundB,verB,worker).Scan(&sumB))
 if sumA != 100 || sumB != 0 { t.Fatalf("unexpected independent round totals: %d / %d",sumA,sumB) }
 _, err = f.db.Pool.Exec(f.ctx,
  "UPDATE bonus_allocation_lines SET final_minor=0 WHERE round_id=$1 AND source_id=$2 AND user_id=$3",
  roundA,sourceA,worker)
 if err == nil { t.Fatal("M2 draft line changed without audited M3 workflow") }
}
