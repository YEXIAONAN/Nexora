-- +goose Up
-- Manual, auditable CNY allocation. No automatic formula and no payment gateway.
-- Saving a draft creates an immutable replacement snapshot; abandoned snapshots,
-- eligibility decisions and adjustments remain available for audit forever.
-- Preserve v6's membership epoch semantics, but guard every UPDATE, including
-- direct epoch writes by an accidentally over-granted application role.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION project_members_bump_authorization() RETURNS trigger AS $$
BEGIN
 IF NEW.authorization_version IS DISTINCT FROM OLD.authorization_version THEN
 RAISE EXCEPTION 'membership authorization version is managed by status transitions' USING ERRCODE='23514'; END IF;
 IF NEW.status IS DISTINCT FROM OLD.status THEN NEW.authorization_version=OLD.authorization_version+1; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
DROP TRIGGER project_members_authorization_guard ON project_members;
CREATE TRIGGER project_members_authorization_guard BEFORE UPDATE ON project_members FOR EACH ROW EXECUTE FUNCTION project_members_bump_authorization();

ALTER TABLE bonus_round_versions ADD COLUMN published_at timestamptz,
 ADD COLUMN published_by uuid REFERENCES users(id) ON DELETE RESTRICT,
 ADD CONSTRAINT bonus_publication_pair CHECK ((published_at IS NULL)=(published_by IS NULL));
ALTER TABLE bonus_allocation_lines ADD COLUMN reason text NOT NULL DEFAULT '' CHECK(length(reason)<=4000);
CREATE TABLE bonus_payments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL, project_id uuid NOT NULL, round_id uuid NOT NULL, version_id uuid NOT NULL,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 amount_minor bigint NOT NULL CHECK(amount_minor BETWEEN 0 AND 900000000000000),
 reference text NOT NULL CHECK(length(btrim(reference)) BETWEEN 1 AND 200),
 recorded_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(version_id,user_id),
 FOREIGN KEY(team_id,project_id,round_id,version_id) REFERENCES bonus_round_versions(team_id,project_id,round_id,id)
);
CREATE TABLE bonus_commands (
 team_id uuid NOT NULL, project_id uuid NOT NULL, actor_id uuid NOT NULL REFERENCES users(id),
 client_request_id uuid NOT NULL, input_digest text NOT NULL CHECK(input_digest ~ '^[0-9a-f]{64}$'),
 round_id uuid NOT NULL, revision integer NOT NULL CHECK(revision>0), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(team_id,project_id,actor_id,client_request_id),
 FOREIGN KEY(team_id,project_id,round_id) REFERENCES bonus_rounds(team_id,project_id,id)
);

-- +goose StatementBegin
CREATE FUNCTION bonus_live(p_team uuid,p_project uuid,p_actor uuid,p_session uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT contribution_member(p_team,p_project,p_actor) AND EXISTS(
 SELECT 1 FROM sessions s JOIN users u ON u.id=s.user_id AND u.is_active WHERE s.id=p_session AND s.user_id=p_actor AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp());
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_manager(p_team uuid,p_project uuid,p_actor uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT contribution_member(p_team,p_project,p_actor) AND (
 EXISTS(SELECT 1 FROM team_members WHERE team_id=p_team AND user_id=p_actor AND status='active' AND role IN ('owner','admin'))
 OR EXISTS(SELECT 1 FROM projects WHERE team_id=p_team AND id=p_project AND lead_id=p_actor));
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_has_role(p_round uuid,p_actor uuid,p_role text) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM bonus_role_grants g JOIN project_members pm
 ON pm.team_id=g.team_id AND pm.project_id=g.project_id AND pm.user_id=g.subject_id
 WHERE g.round_id=p_round AND g.subject_id=p_actor AND (p_role IS NULL OR g.role_code=p_role)
 AND g.status='active' AND g.grant_mode='internal' AND g.member_authorization_version=pm.authorization_version
 AND contribution_member(g.team_id,g.project_id,p_actor));
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_can_edit(p_round uuid,p_actor uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT bonus_has_role(p_round,p_actor,'editor') AND bonus_has_role(p_round,p_actor,'budget') AND bonus_has_role(p_round,p_actor,'eligibility');
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_source_current(p_confirmation uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM contribution_confirmations f
 JOIN contribution_case_versions v ON v.id=f.case_version_id
 JOIN contribution_cases c ON c.id=v.case_id
 JOIN tasks t ON t.id=c.task_id AND t.team_id=c.team_id AND t.project_id=c.project_id
 JOIN task_reviews tr ON tr.id=c.review_id AND tr.task_id=t.id AND tr.submission_version_id=c.submission_version_id
 WHERE f.id=p_confirmation AND c.effective_confirmed_version_id=v.id AND c.case_workflow_status='confirmed'
 AND v.status='confirmed' AND NOT contribution_open_disputes(c.id)
 AND contribution_complete_shares(v.id) AND f.version_digest=contribution_digest(v.id)
 AND t.status='completed' AND t.accepted_review_id=c.review_id AND t.current_submission_version_id=c.submission_version_id AND tr.decision='approve');
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_independent(p_version uuid,p_actor uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM bonus_round_versions v JOIN bonus_rounds r ON r.id=v.round_id
 WHERE v.id=p_version AND bonus_has_role(r.id,p_actor,'confirmer')
 AND NOT bonus_has_role(r.id,p_actor,'editor') AND NOT bonus_has_role(r.id,p_actor,'budget') AND NOT bonus_has_role(r.id,p_actor,'eligibility')
 AND r.created_by<>p_actor AND v.proposed_by<>p_actor
 -- Saving immutable draft snapshots must not launder a former allocator into
 -- an independent reviewer. Retain round-wide authorship conflicts.
 AND NOT EXISTS(SELECT 1 FROM bonus_round_versions prior WHERE prior.round_id=r.id AND prior.proposed_by=p_actor)
 AND NOT EXISTS(SELECT 1 FROM bonus_adjustments a WHERE a.round_id=r.id AND a.actor_id=p_actor)
 AND NOT EXISTS(SELECT 1 FROM bonus_eligibility_decisions e WHERE e.round_id=r.id AND e.decided_by=p_actor)
 AND NOT EXISTS(SELECT 1 FROM bonus_round_sources s
 JOIN contribution_case_versions cv ON cv.id=s.case_version_id JOIN contribution_confirmations cf ON cf.id=s.confirmation_id
 WHERE s.version_id=v.id AND (cv.proposed_by=p_actor OR cf.confirmed_by=p_actor
 OR EXISTS(SELECT 1 FROM contribution_shares cs WHERE cs.case_version_id=cv.id AND cs.user_id=p_actor)
 OR EXISTS(SELECT 1 FROM contribution_conflicts cc WHERE cc.case_id=cv.case_id AND cc.user_id=p_actor))));
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_plan_json(p_version uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('id',v.id,'version',v.version,'status',v.status,'total_budget_minor',v.total_budget_minor,
 'plan_unallocated_minor',v.plan_unallocated_minor,'rationale',v.rationale,'published_at',v.published_at,
 'sources',COALESCE((SELECT jsonb_agg(jsonb_build_object('confirmation_id',s.confirmation_id,'task_title',t.title,
 'source_budget_minor',s.source_budget_minor,'source_unallocated_minor',s.source_unallocated_minor,'reason',s.reason,
 'lines',COALESCE((SELECT jsonb_agg(jsonb_build_object('user_id',l.user_id,'display_name',u.display_name,'share_bps',l.share_bps,
 'eligible',e.decision='eligible','amount_minor',l.final_minor,'reason',l.reason) ORDER BY l.user_id)
 FROM bonus_allocation_lines l JOIN users u ON u.id=l.user_id JOIN bonus_eligibility_decisions e ON e.id=l.eligibility_decision_id
 WHERE l.source_id=s.id),'[]'::jsonb)) ORDER BY s.confirmation_id) FROM bonus_round_sources s JOIN tasks t ON t.id=s.task_id WHERE s.version_id=v.id),'[]'::jsonb))
 FROM bonus_round_versions v WHERE v.id=p_version;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_digest(p_version uuid) RETURNS text LANGUAGE sql STABLE AS $$
 -- Names/titles, publication and workflow state are presentation, not money evidence.
 SELECT encode(sha256(convert_to(jsonb_build_object('id',v.id,'proposed_by',v.proposed_by,'budget',v.total_budget_minor,
 'unallocated',v.plan_unallocated_minor,'rationale',v.rationale,
 'sources',COALESCE((SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM bonus_round_sources s WHERE s.version_id=v.id),'[]'::jsonb),
 'eligibility',COALESCE((SELECT jsonb_agg(to_jsonb(e) ORDER BY e.user_id) FROM bonus_eligibility_decisions e WHERE e.version_id=v.id),'[]'::jsonb),
 'lines',COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.source_id,l.user_id) FROM bonus_allocation_lines l WHERE l.version_id=v.id),'[]'::jsonb))::text,'UTF8')),'hex')
 FROM bonus_round_versions v WHERE v.id=p_version;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_validate_plan(p_version uuid,p_require_sources boolean) RETURNS void LANGUAGE plpgsql AS $$
DECLARE v bonus_round_versions%ROWTYPE; s bonus_round_sources%ROWTYPE; snapshot jsonb;
BEGIN
 SELECT * INTO v FROM bonus_round_versions WHERE id=p_version;
 IF v.id IS NULL OR (p_require_sources AND NOT EXISTS(SELECT 1 FROM bonus_round_sources WHERE version_id=v.id)) THEN
 RAISE EXCEPTION 'nonempty plan required' USING ERRCODE='23514'; END IF;
 IF v.total_budget_minor <> v.plan_unallocated_minor + COALESCE((SELECT sum(source_budget_minor) FROM bonus_round_sources WHERE version_id=v.id),0) THEN
 RAISE EXCEPTION 'plan budget does not conserve funds' USING ERRCODE='23514'; END IF;
 FOR s IN SELECT * FROM bonus_round_sources WHERE version_id=v.id LOOP
 IF NOT bonus_source_current(s.confirmation_id) OR NOT EXISTS(SELECT 1 FROM contribution_confirmations WHERE id=s.confirmation_id AND version_digest=s.confirmation_digest) THEN
 RAISE EXCEPTION 'contribution source is no longer effective' USING ERRCODE='23514'; END IF;
 SELECT jsonb_agg(jsonb_build_object('user_id',user_id,'share_bps',share_bps) ORDER BY user_id) INTO snapshot FROM contribution_shares WHERE case_version_id=s.case_version_id;
 IF s.shares_snapshot IS DISTINCT FROM snapshot OR s.coverage_decision<>'included'
 OR s.source_budget_minor<>s.source_unallocated_minor+COALESCE((SELECT sum(final_minor) FROM bonus_allocation_lines WHERE source_id=s.id),0)
 OR (SELECT count(*) FROM bonus_allocation_lines WHERE source_id=s.id)<>(SELECT count(*) FROM contribution_shares WHERE case_version_id=s.case_version_id)
 OR EXISTS(SELECT 1 FROM contribution_shares cs LEFT JOIN bonus_allocation_lines l ON l.source_id=s.id AND l.user_id=cs.user_id WHERE cs.case_version_id=s.case_version_id AND (l.user_id IS NULL OR l.share_bps<>cs.share_bps)) THEN
 RAISE EXCEPTION 'source conservation or complete official shares required' USING ERRCODE='23514'; END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM bonus_allocation_lines l JOIN bonus_eligibility_decisions e ON e.id=l.eligibility_decision_id
 JOIN project_members pm ON pm.team_id=e.team_id AND pm.project_id=e.project_id AND pm.user_id=e.user_id
 WHERE l.version_id=v.id AND (l.raw_rounded_minor<>0 OR l.suggested_minor<>0 OR length(btrim(l.reason))=0
 OR e.decided_by=e.user_id OR (e.decision='ineligible' AND l.final_minor<>0)
 OR (e.decision='eligible' AND (NOT contribution_member(e.team_id,e.project_id,e.user_id) OR e.member_authorization_version<>pm.authorization_version)))) THEN
 RAISE EXCEPTION 'manual amounts and current explicit eligibility required' USING ERRCODE='23514'; END IF;
END;
$$;
-- +goose StatementEnd

-- Keep source/line evidence and all historical facts append-only. Replace only
-- the M2 gates that prevented the authorized workflow, not the archive guard.
DROP TRIGGER bonus_m2_lock ON bonus_rounds;
DROP TRIGGER bonus_m2_lock ON bonus_round_versions;
DROP TRIGGER bonus_m2_lock ON bonus_role_grants;
DROP TRIGGER bonus_m2_version_finalization ON bonus_round_versions;
DROP TRIGGER bonus_m2_grant_activation ON bonus_role_grants;
DROP TRIGGER bonus_m2_confirmation_disabled ON bonus_confirmations;
-- +goose StatementBegin
CREATE FUNCTION bonus_workflow_version_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'bonus history is immutable' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' THEN
 IF NEW.status<>'draft' OR NEW.published_at IS NOT NULL THEN RAISE EXCEPTION 'versions start as unpublished drafts' USING ERRCODE='23514'; END IF;
 ELSE
 IF NOT (OLD.status='confirmed' AND NEW.status='confirmed' AND OLD.published_at IS NULL AND NEW.published_at IS NOT NULL)
 AND (NEW.published_at IS DISTINCT FROM OLD.published_at OR NEW.published_by IS DISTINCT FROM OLD.published_by) THEN RAISE EXCEPTION 'publication requires its own transition' USING ERRCODE='23514'; END IF;
 IF NOT ((OLD.status='draft' AND NEW.status='pending_review') OR (OLD.status='pending_review' AND NEW.status='draft'))
 AND (NEW.submitted_at IS DISTINCT FROM OLD.submitted_at OR NEW.submitted_digest IS DISTINCT FROM OLD.submitted_digest) THEN RAISE EXCEPTION 'submitted evidence is immutable' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-ARRAY['status','submitted_at','submitted_digest','published_at','published_by']) IS DISTINCT FROM
 (to_jsonb(OLD)-ARRAY['status','submitted_at','submitted_digest','published_at','published_by']) THEN RAISE EXCEPTION 'save a new snapshot, never overwrite money evidence' USING ERRCODE='23514'; END IF;
 IF OLD.status='draft' AND NEW.status IN ('pending_review','abandoned') THEN
 IF NEW.status='pending_review' THEN
 PERFORM bonus_validate_plan(NEW.id,true);
 NEW.submitted_at:=clock_timestamp(); NEW.submitted_digest:=bonus_digest(NEW.id);
 END IF;
 ELSIF OLD.status='pending_review' AND NEW.status='draft' THEN NEW.submitted_at:=NULL; NEW.submitted_digest:=NULL;
 ELSIF OLD.status='pending_review' AND NEW.status='confirmed' THEN
 IF NOT EXISTS(SELECT 1 FROM bonus_confirmations WHERE version_id=NEW.id AND version_digest=OLD.submitted_digest) THEN RAISE EXCEPTION 'confirmation fact required' USING ERRCODE='23514'; END IF;
 ELSIF OLD.status='confirmed' AND NEW.status='superseded' THEN
 IF NOT EXISTS(SELECT 1 FROM bonus_confirmations f JOIN bonus_round_versions v ON v.id=f.version_id WHERE v.round_id=NEW.round_id AND v.version>NEW.version AND v.status='confirmed') THEN RAISE EXCEPTION 'new confirmation required' USING ERRCODE='23514'; END IF;
 IF EXISTS(SELECT 1 FROM bonus_payments WHERE version_id=OLD.id) THEN RAISE EXCEPTION 'recorded payments prevent revision' USING ERRCODE='23514'; END IF;
 ELSIF OLD.status='confirmed' AND NEW.status='confirmed' AND OLD.published_at IS NULL AND NEW.published_at IS NOT NULL THEN
 PERFORM bonus_validate_plan(NEW.id,true);
 IF NOT bonus_has_role(NEW.round_id,NEW.published_by,'editor') OR NOT bonus_has_role(NEW.round_id,NEW.published_by,'budget') THEN RAISE EXCEPTION 'publication requires explicit authority' USING ERRCODE='42501'; END IF;
 ELSE RAISE EXCEPTION 'illegal bonus transition' USING ERRCODE='23514'; END IF;
 IF OLD.published_at IS NOT NULL AND (NEW.published_at IS DISTINCT FROM OLD.published_at OR NEW.published_by IS DISTINCT FROM OLD.published_by) THEN RAISE EXCEPTION 'publication is immutable' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER bonus_workflow_version BEFORE INSERT OR UPDATE OR DELETE ON bonus_round_versions FOR EACH ROW EXECUTE FUNCTION bonus_workflow_version_guard();
-- +goose StatementBegin
CREATE FUNCTION bonus_workflow_round_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'funding identity is immutable' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-ARRAY['revision','latest_version_id','effective_confirmed_version_id']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['revision','latest_version_id','effective_confirmed_version_id']) OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'round identity or revision cannot be overwritten' USING ERRCODE='23514'; END IF;
 IF NEW.latest_version_id IS NULL OR NOT EXISTS(SELECT 1 FROM bonus_round_versions WHERE id=NEW.latest_version_id AND round_id=NEW.id) THEN RAISE EXCEPTION 'real latest version required' USING ERRCODE='23514'; END IF;
 IF NEW.effective_confirmed_version_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM bonus_confirmations f JOIN bonus_round_versions v ON v.id=f.version_id WHERE v.id=NEW.effective_confirmed_version_id AND v.round_id=NEW.id AND v.status='confirmed') THEN RAISE EXCEPTION 'effective pointer requires confirmed evidence' USING ERRCODE='23514'; END IF;
 IF OLD.effective_confirmed_version_id IS NOT NULL AND NEW.effective_confirmed_version_id IS DISTINCT FROM OLD.effective_confirmed_version_id AND EXISTS(SELECT 1 FROM bonus_payments WHERE round_id=NEW.id) THEN RAISE EXCEPTION 'recorded payments prevent replacement' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER bonus_workflow_round BEFORE UPDATE OR DELETE ON bonus_rounds FOR EACH ROW EXECUTE FUNCTION bonus_workflow_round_guard();
-- +goose StatementBegin
CREATE FUNCTION bonus_workflow_grant_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'grant evidence cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
 IF OLD.status<>'active' OR NEW.status<>'revoked' OR (to_jsonb(OLD)-'status') IS DISTINCT FROM (to_jsonb(NEW)-'status') THEN RAISE EXCEPTION 'only revocation is allowed; regrant a new epoch' USING ERRCODE='23514'; END IF;
 ELSE
 IF NEW.status<>'active' OR NEW.grant_mode<>'internal' OR NOT bonus_manager(NEW.team_id,NEW.project_id,NEW.granted_by)
 OR NOT contribution_member(NEW.team_id,NEW.project_id,NEW.subject_id)
 OR NEW.member_authorization_version IS DISTINCT FROM (SELECT authorization_version FROM project_members WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND user_id=NEW.subject_id) THEN RAISE EXCEPTION 'explicit active internal grant required' USING ERRCODE='23514'; END IF;
 IF (NEW.role_code='confirmer' AND (bonus_has_role(NEW.round_id,NEW.subject_id,'editor') OR bonus_has_role(NEW.round_id,NEW.subject_id,'budget') OR bonus_has_role(NEW.round_id,NEW.subject_id,'eligibility')))
 OR (NEW.role_code IN ('editor','budget','eligibility') AND bonus_has_role(NEW.round_id,NEW.subject_id,'confirmer')) THEN RAISE EXCEPTION 'review and allocation authorities must be separate' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER bonus_workflow_grant BEFORE INSERT OR UPDATE OR DELETE ON bonus_role_grants FOR EACH ROW EXECUTE FUNCTION bonus_workflow_grant_guard();
-- +goose StatementBegin
CREATE FUNCTION bonus_workflow_confirmation_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE v bonus_round_versions%ROWTYPE;
BEGIN
 PERFORM 1 FROM teams WHERE id=NEW.team_id FOR UPDATE;
 SELECT * INTO v FROM bonus_round_versions WHERE id=NEW.version_id;
 IF v.status IS DISTINCT FROM 'pending_review' OR NOT bonus_independent(v.id,NEW.confirmed_by)
 OR NOT EXISTS(SELECT 1 FROM bonus_rounds WHERE id=NEW.round_id AND latest_version_id=v.id)
 OR NOT EXISTS(SELECT 1 FROM bonus_role_grants WHERE id=NEW.grant_id AND round_id=NEW.round_id AND subject_id=NEW.confirmed_by AND role_code='confirmer' AND status='active' AND grant_mode='internal' AND member_authorization_version=(SELECT authorization_version FROM project_members WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND user_id=NEW.confirmed_by))
 OR v.submitted_digest IS DISTINCT FROM bonus_digest(v.id) OR NEW.version_digest IS DISTINCT FROM v.submitted_digest THEN RAISE EXCEPTION 'independent current confirmer and untampered submitted plan required' USING ERRCODE='23514'; END IF;
 PERFORM bonus_validate_plan(v.id,true);
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER bonus_workflow_confirmation BEFORE INSERT ON bonus_confirmations FOR EACH ROW EXECUTE FUNCTION bonus_workflow_confirmation_guard();
-- +goose StatementBegin
CREATE FUNCTION bonus_workflow_payment_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'simulated payment records are append-only' USING ERRCODE='23514'; END IF;
 PERFORM 1 FROM teams WHERE id=NEW.team_id FOR UPDATE;
 IF NOT bonus_has_role(NEW.round_id,NEW.recorded_by,'editor') OR NOT bonus_has_role(NEW.round_id,NEW.recorded_by,'budget')
 OR NOT contribution_member(NEW.team_id,NEW.project_id,NEW.user_id)
 OR NOT EXISTS(SELECT 1 FROM bonus_rounds r JOIN bonus_round_versions v ON v.id=r.effective_confirmed_version_id
 WHERE r.id=NEW.round_id AND r.latest_version_id=v.id AND v.id=NEW.version_id AND v.status='confirmed' AND v.published_at IS NOT NULL)
 OR NEW.amount_minor IS DISTINCT FROM (SELECT total_minor::bigint FROM bonus_member_totals WHERE version_id=NEW.version_id AND user_id=NEW.user_id) THEN RAISE EXCEPTION 'payment must record the full current published allocation with no revision in progress' USING ERRCODE='23514'; END IF;
 PERFORM bonus_validate_plan(NEW.version_id,true);
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER bonus_workflow_payment BEFORE INSERT OR UPDATE OR DELETE ON bonus_payments FOR EACH ROW EXECUTE FUNCTION bonus_workflow_payment_guard();
CREATE TRIGGER bonus_workflow_command_immutable BEFORE UPDATE OR DELETE ON bonus_commands FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();
-- +goose StatementBegin
CREATE FUNCTION bonus_workflow_detail_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM bonus_round_versions WHERE id=NEW.version_id AND status='draft') THEN RAISE EXCEPTION 'details require a draft' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER bonus_workflow_line_draft BEFORE INSERT ON bonus_allocation_lines FOR EACH ROW EXECUTE FUNCTION bonus_workflow_detail_guard();
CREATE TRIGGER bonus_workflow_eligibility_draft BEFORE INSERT ON bonus_eligibility_decisions FOR EACH ROW EXECUTE FUNCTION bonus_workflow_detail_guard();
CREATE TRIGGER bonus_workflow_adjustment_draft BEFORE INSERT ON bonus_adjustments FOR EACH ROW EXECUTE FUNCTION bonus_workflow_detail_guard();
-- +goose StatementBegin
CREATE FUNCTION bonus_minor(p_value jsonb) RETURNS bigint LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE n bigint;
BEGIN
 IF p_value IS NULL OR jsonb_typeof(p_value)<>'number' OR p_value::text !~ '^(0|[1-9][0-9]*)$' THEN RAISE EXCEPTION 'money must be integer CNY minor units' USING ERRCODE='22023'; END IF;
 n:=p_value::text::bigint;
 IF n NOT BETWEEN 0 AND 900000000000000 THEN RAISE EXCEPTION 'money out of range' USING ERRCODE='22023'; END IF;
 RETURN n;
END;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_grant_internal(p_team uuid,p_project uuid,p_round uuid,p_subject uuid,p_role text,p_actor uuid,p_reason text,p_request text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF bonus_has_role(p_round,p_subject,p_role) THEN RAISE EXCEPTION 'grant already active' USING ERRCODE='23505'; END IF;
 INSERT INTO bonus_role_grants(team_id,project_id,round_id,subject_id,role_code,grant_mode,status,member_authorization_version,granted_by,reason,request_id)
 SELECT p_team,p_project,p_round,p_subject,p_role,'internal','active',authorization_version,p_actor,p_reason,p_request
 FROM project_members WHERE team_id=p_team AND project_id=p_project AND user_id=p_subject;
 IF NOT FOUND THEN RAISE EXCEPTION 'grant subject must be a project member' USING ERRCODE='23514'; END IF;
END;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_save_snapshot(p_round uuid,p_actor uuid,p_input jsonb) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE r bonus_rounds%ROWTYPE; oldv bonus_round_versions%ROWTYPE; v uuid:=gen_random_uuid();
 src jsonb; ln jsonb; f contribution_confirmations%ROWTYPE; sid uuid; uid uuid; eid uuid; eligibility_value text;
 total bigint; budgets numeric:=0; allocated numeric; sb bigint; amount bigint; old_amount bigint; old_balance bigint; bps integer;
BEGIN
 SELECT * INTO r FROM bonus_rounds WHERE id=p_round;
 SELECT * INTO oldv FROM bonus_round_versions WHERE id=r.latest_version_id;
 total:=bonus_minor(p_input->'total_budget_minor');
 IF jsonb_typeof(p_input->'sources') IS DISTINCT FROM 'array' OR jsonb_array_length(p_input->'sources')>200 THEN RAISE EXCEPTION 'bounded sources array required' USING ERRCODE='22023'; END IF;
 FOR src IN SELECT value FROM jsonb_array_elements(p_input->'sources') LOOP budgets:=budgets+bonus_minor(src->'source_budget_minor'); END LOOP;
 IF budgets>total THEN RAISE EXCEPTION 'source budgets exceed total budget' USING ERRCODE='23514'; END IF;
 UPDATE bonus_round_versions SET status='abandoned' WHERE id=oldv.id;
 INSERT INTO bonus_round_versions(id,team_id,project_id,round_id,version,previous_version_id,total_budget_minor,plan_unallocated_minor,proposed_by,rationale,client_request_id)
 VALUES(v,r.team_id,r.project_id,r.id,oldv.version+1,oldv.id,total,total-budgets,p_actor,p_input->>'reason',(p_input->>'client_request_id')::uuid);
 FOR src IN SELECT value FROM jsonb_array_elements(p_input->'sources') LOOP
 SELECT * INTO f FROM contribution_confirmations WHERE id=(src->>'confirmation_id')::uuid AND team_id=r.team_id AND project_id=r.project_id;
 IF f.id IS NULL OR NOT bonus_source_current(f.id) THEN RAISE EXCEPTION 'current scoped contribution required' USING ERRCODE='23514'; END IF;
 IF jsonb_typeof(src->'lines') IS DISTINCT FROM 'array' OR jsonb_array_length(src->'lines')>500 THEN RAISE EXCEPTION 'bounded explicit lines required' USING ERRCODE='22023'; END IF;
 sb:=bonus_minor(src->'source_budget_minor'); allocated:=0;
 FOR ln IN SELECT value FROM jsonb_array_elements(src->'lines') LOOP allocated:=allocated+bonus_minor(ln->'amount_minor'); END LOOP;
 IF allocated>sb THEN RAISE EXCEPTION 'allocations exceed source budget' USING ERRCODE='23514'; END IF;
 sid:=gen_random_uuid();
 INSERT INTO bonus_round_sources(id,team_id,project_id,round_id,version_id,task_id,case_id,case_version_id,confirmation_id,confirmation_digest,coverage_decision,source_budget_minor,source_unallocated_minor,shares_snapshot,reason)
 VALUES(sid,r.team_id,r.project_id,r.id,v,f.task_id,f.case_id,f.case_version_id,f.id,f.version_digest,'included',sb,sb-allocated,
 (SELECT jsonb_agg(jsonb_build_object('user_id',user_id,'share_bps',share_bps) ORDER BY user_id) FROM contribution_shares WHERE case_version_id=f.case_version_id),src->>'reason');
 FOR ln IN SELECT value FROM jsonb_array_elements(src->'lines') LOOP
 uid:=(ln->>'user_id')::uuid; amount:=bonus_minor(ln->'amount_minor');
 IF jsonb_typeof(ln->'eligible') IS DISTINCT FROM 'boolean' OR uid=p_actor OR length(btrim(COALESCE(ln->>'reason','')))=0 THEN RAISE EXCEPTION 'explicit non-self eligibility and reason required' USING ERRCODE='23514'; END IF;
 eligibility_value:=CASE WHEN (ln->>'eligible')::boolean THEN 'eligible' ELSE 'ineligible' END;
 SELECT id INTO eid FROM bonus_eligibility_decisions WHERE version_id=v AND user_id=uid;
 IF eid IS NULL THEN
 INSERT INTO bonus_eligibility_decisions(team_id,project_id,round_id,version_id,user_id,decision,member_authorization_version,decided_by,reason)
 SELECT r.team_id,r.project_id,r.id,v,uid,eligibility_value,authorization_version,p_actor,ln->>'reason' FROM project_members WHERE team_id=r.team_id AND project_id=r.project_id AND user_id=uid RETURNING id INTO eid;
 ELSE
 IF NOT EXISTS(SELECT 1 FROM bonus_eligibility_decisions WHERE id=eid AND bonus_eligibility_decisions.decision=eligibility_value) THEN RAISE EXCEPTION 'eligibility must agree across sources' USING ERRCODE='23514'; END IF;
 END IF;
 SELECT share_bps INTO bps FROM contribution_shares WHERE case_version_id=f.case_version_id AND user_id=uid;
 INSERT INTO bonus_allocation_lines(team_id,project_id,round_id,version_id,source_id,case_version_id,user_id,eligibility_decision_id,share_bps,raw_rounded_minor,suggested_minor,final_minor,reason)
 VALUES(r.team_id,r.project_id,r.id,v,sid,f.case_version_id,uid,eid,bps,0,0,amount,ln->>'reason');
 SELECT l.final_minor,s.source_unallocated_minor INTO old_amount,old_balance FROM bonus_round_sources s JOIN bonus_allocation_lines l ON l.source_id=s.id WHERE s.version_id=oldv.id AND s.confirmation_id=f.id AND l.user_id=uid;
 INSERT INTO bonus_adjustments(team_id,project_id,round_id,version_id,source_id,user_id,old_minor,new_minor,delta_minor,old_source_unallocated_minor,new_source_unallocated_minor,actor_id,reason,request_id)
 VALUES(r.team_id,r.project_id,r.id,v,sid,uid,COALESCE(old_amount,0),amount,amount-COALESCE(old_amount,0),COALESCE(old_balance,0),sb-allocated,p_actor,src->>'reason',(p_input->>'client_request_id')||':'||f.id||':'||uid);
 END LOOP;
 END LOOP;
 PERFORM bonus_validate_plan(v,false);
 RETURN v;
END;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_workflow_command(p_team uuid,p_project uuid,p_round uuid,p_actor uuid,p_session uuid,p_action text,p_input jsonb) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER AS $$
DECLARE r bonus_rounds%ROWTYPE; v bonus_round_versions%ROWTYPE; receipt bonus_commands%ROWTYPE;
 key uuid; digest text; reason text; target uuid; nextv uuid; manager uuid; reviewer uuid; role text; grant_id uuid; amount bigint; result jsonb;
BEGIN
 IF current_setting('transaction_isolation')<>'read committed' THEN RAISE EXCEPTION 'bonus commands require read committed' USING ERRCODE='25001'; END IF;
 PERFORM 1 FROM teams WHERE id=p_team FOR UPDATE;
 IF NOT bonus_live(p_team,p_project,p_actor,p_session) THEN RAISE EXCEPTION 'resource not found' USING ERRCODE='P0002'; END IF;
 -- Prevent session revocation during a command; membership changes use the team lock.
 PERFORM 1 FROM sessions WHERE id=p_session AND user_id=p_actor AND revoked_at IS NULL AND expires_at>clock_timestamp() FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'resource not found' USING ERRCODE='P0002'; END IF;
 IF p_action NOT IN ('create','plan','submit','return','confirm','publish','revise','payments','grants') THEN RAISE EXCEPTION 'unknown command' USING ERRCODE='22023'; END IF;
 IF p_action IN ('create','grants') THEN
 IF NOT bonus_manager(p_team,p_project,p_actor) THEN RAISE EXCEPTION 'project management authority required' USING ERRCODE='42501'; END IF;
 END IF;
 IF p_action<>'create' THEN
 SELECT * INTO r FROM bonus_rounds WHERE team_id=p_team AND project_id=p_project AND id=p_round FOR UPDATE;
 IF r.id IS NULL THEN RAISE EXCEPTION 'resource not found' USING ERRCODE='P0002'; END IF;
 IF p_action<>'grants' AND NOT bonus_has_role(r.id,p_actor,NULL) THEN RAISE EXCEPTION 'explicit bonus grant required' USING ERRCODE='42501'; END IF;
 IF p_action IN ('plan','submit','revise') AND NOT bonus_can_edit(r.id,p_actor) THEN RAISE EXCEPTION 'editor budget and eligibility grants required' USING ERRCODE='42501'; END IF;
 IF p_action IN ('publish','payments') AND NOT (bonus_has_role(r.id,p_actor,'editor') AND bonus_has_role(r.id,p_actor,'budget')) THEN RAISE EXCEPTION 'editor and budget grants required' USING ERRCODE='42501'; END IF;
 IF p_action IN ('return','confirm') AND NOT bonus_has_role(r.id,p_actor,'confirmer') THEN RAISE EXCEPTION 'confirmer grant required' USING ERRCODE='42501'; END IF;
 END IF;
 key:=(p_input->>'client_request_id')::uuid; reason:=btrim(p_input->>'reason');
 IF key IS NULL OR key='00000000-0000-0000-0000-000000000000'::uuid OR reason IS NULL OR length(reason) NOT BETWEEN 1 AND 4000 THEN RAISE EXCEPTION 'request identity and reason required' USING ERRCODE='22023'; END IF;
 digest:=encode(sha256(convert_to(jsonb_build_object('round',p_round,'action',p_action,'input',p_input)::text,'UTF8')),'hex');
 SELECT * INTO receipt FROM bonus_commands WHERE team_id=p_team AND project_id=p_project AND actor_id=p_actor AND client_request_id=key;
 IF FOUND THEN
 IF receipt.input_digest<>digest THEN RAISE EXCEPTION 'idempotency key reused with different input' USING ERRCODE='40001'; END IF;
 RETURN jsonb_build_object('round_id',receipt.round_id,'revision',receipt.revision);
 END IF;
 IF p_action='create' THEN
 manager:=(p_input->>'manager_id')::uuid; reviewer:=(p_input->>'reviewer_id')::uuid;
 IF manager IS NULL OR reviewer IS NULL OR manager=reviewer OR manager=p_actor OR reviewer=p_actor THEN RAISE EXCEPTION 'choose two distinct non-self internal officers' USING ERRCODE='23514'; END IF;
 INSERT INTO bonus_rounds(team_id,project_id,funding_key,funding_label,funding_source,created_by)
 VALUES(p_team,p_project,p_input->>'funding_key',p_input->>'funding_label',p_input->>'funding_source',p_actor) RETURNING * INTO r;
 INSERT INTO bonus_round_versions(team_id,project_id,round_id,version,proposed_by,rationale,client_request_id)
 VALUES(p_team,p_project,r.id,1,manager,reason,key) RETURNING id INTO nextv;
 FOREACH role IN ARRAY ARRAY['editor','budget','eligibility'] LOOP PERFORM bonus_grant_internal(p_team,p_project,r.id,manager,role,p_actor,reason,key||':'||role); END LOOP;
 FOREACH role IN ARRAY ARRAY['confirmer','auditor'] LOOP PERFORM bonus_grant_internal(p_team,p_project,r.id,reviewer,role,p_actor,reason,key||':'||role); END LOOP;
 ELSE
 IF jsonb_typeof(p_input->'expected_revision') IS DISTINCT FROM 'number' OR (p_input->>'expected_revision') !~ '^[1-9][0-9]*$' OR r.revision IS DISTINCT FROM (p_input->>'expected_revision')::integer THEN RAISE EXCEPTION 'revision conflict' USING ERRCODE='40001'; END IF;
 SELECT * INTO v FROM bonus_round_versions WHERE id=r.latest_version_id;
 nextv:=v.id;
 CASE p_action
 WHEN 'plan' THEN
 IF v.status IS DISTINCT FROM 'draft' THEN RAISE EXCEPTION 'only draft can be saved' USING ERRCODE='23514'; END IF;
 nextv:=bonus_save_snapshot(r.id,p_actor,p_input);
 WHEN 'submit' THEN
 IF v.status IS DISTINCT FROM 'draft' THEN RAISE EXCEPTION 'only draft can be submitted' USING ERRCODE='23514'; END IF;
 UPDATE bonus_round_versions SET status='pending_review' WHERE id=v.id;
 WHEN 'return' THEN
 IF v.status IS DISTINCT FROM 'pending_review' THEN RAISE EXCEPTION 'only pending review can be returned' USING ERRCODE='23514'; END IF;
 UPDATE bonus_round_versions SET status='draft' WHERE id=v.id;
 WHEN 'confirm' THEN
 IF NOT bonus_independent(v.id,p_actor) THEN RAISE EXCEPTION 'independent confirmer required' USING ERRCODE='42501'; END IF;
 SELECT g.id INTO grant_id FROM bonus_role_grants g JOIN project_members pm ON pm.team_id=g.team_id AND pm.project_id=g.project_id AND pm.user_id=g.subject_id
 WHERE g.round_id=r.id AND g.subject_id=p_actor AND g.role_code='confirmer' AND g.status='active' AND g.grant_mode='internal' AND g.member_authorization_version=pm.authorization_version ORDER BY g.created_at DESC LIMIT 1;
 INSERT INTO bonus_confirmations(team_id,project_id,round_id,version_id,grant_id,confirmed_by,version_digest,reason,request_id)
 VALUES(p_team,p_project,r.id,v.id,grant_id,p_actor,v.submitted_digest,reason,key::text);
 UPDATE bonus_round_versions SET status='confirmed' WHERE id=v.id;
 IF r.effective_confirmed_version_id IS NOT NULL THEN UPDATE bonus_round_versions SET status='superseded' WHERE id=r.effective_confirmed_version_id; END IF;
 r.effective_confirmed_version_id:=v.id;
 WHEN 'publish' THEN
 IF v.status IS DISTINCT FROM 'confirmed' OR v.published_at IS NOT NULL OR r.effective_confirmed_version_id IS DISTINCT FROM v.id THEN RAISE EXCEPTION 'unpublished effective confirmation required' USING ERRCODE='23514'; END IF;
 UPDATE bonus_round_versions SET published_at=clock_timestamp(),published_by=p_actor WHERE id=v.id;
 WHEN 'revise' THEN
 IF v.status IS DISTINCT FROM 'confirmed' OR EXISTS(SELECT 1 FROM bonus_payments WHERE round_id=r.id) THEN RAISE EXCEPTION 'only unpaid confirmed rounds can be revised' USING ERRCODE='23514'; END IF;
 INSERT INTO bonus_round_versions(team_id,project_id,round_id,version,previous_version_id,proposed_by,rationale,client_request_id)
 VALUES(p_team,p_project,r.id,v.version+1,v.id,p_actor,reason,key) RETURNING id INTO nextv;
 WHEN 'payments' THEN
 target:=(p_input->>'user_id')::uuid;
 SELECT total_minor::bigint INTO amount FROM bonus_member_totals WHERE version_id=r.effective_confirmed_version_id AND user_id=target;
 IF amount IS NULL THEN RAISE EXCEPTION 'published allocation recipient required' USING ERRCODE='23514'; END IF;
 INSERT INTO bonus_payments(team_id,project_id,round_id,version_id,user_id,amount_minor,reference,recorded_by,reason)
 VALUES(p_team,p_project,r.id,r.effective_confirmed_version_id,target,amount,p_input->>'reference',p_actor,reason);
 WHEN 'grants' THEN
 target:=(p_input->>'subject_id')::uuid; role:=p_input->>'role_code';
 IF target IS NULL OR target=p_actor OR role NOT IN ('editor','budget','eligibility','confirmer','auditor') OR role IS NULL OR jsonb_typeof(p_input->'revoke') IS DISTINCT FROM 'boolean' THEN RAISE EXCEPTION 'explicit non-self grant target and role required' USING ERRCODE='23514'; END IF;
 IF (p_input->>'revoke')::boolean THEN
 UPDATE bonus_role_grants SET status='revoked' WHERE round_id=r.id AND subject_id=target AND role_code=role AND status='active';
 IF NOT FOUND THEN RAISE EXCEPTION 'active grant not found' USING ERRCODE='23514'; END IF;
 ELSE PERFORM bonus_grant_internal(p_team,p_project,r.id,target,role,p_actor,reason,key::text);
 END IF;
 END CASE;
 END IF;
 UPDATE bonus_rounds SET latest_version_id=nextv,effective_confirmed_version_id=r.effective_confirmed_version_id,revision=revision+1 WHERE id=r.id RETURNING revision INTO r.revision;
 INSERT INTO bonus_events(team_id,project_id,round_id,version_id,actor_id,event_type,reason,details,request_id)
 VALUES(p_team,p_project,r.id,nextv,p_actor,p_action,reason,jsonb_build_object('previous_version_id',v.id,'revision',r.revision,'subject_id',target,'role_code',role),key::text);
 -- General audit intentionally contains no amounts or eligibility decisions.
 INSERT INTO audit_events(actor_id,action,target_type,target_id,team_id,request_id,details)
 VALUES(p_actor,'bonus.'||p_action,'bonus_round',r.id,p_team,key::text,jsonb_build_object('project_id',p_project,'version_id',nextv,'revision',r.revision));
 INSERT INTO bonus_commands(team_id,project_id,actor_id,client_request_id,input_digest,round_id,revision) VALUES(p_team,p_project,p_actor,key,digest,r.id,r.revision);
 result:=jsonb_build_object('round_id',r.id,'revision',r.revision);
 RETURN result;
END;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_metadata_version(p_version uuid,p_effective boolean) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('version',v.version,'status',v.status,'created_at',v.created_at,'submitted_at',v.submitted_at,'confirmed_at',f.confirmed_at,'effective',p_effective)
 FROM bonus_round_versions v LEFT JOIN bonus_confirmations f ON f.version_id=v.id WHERE v.id=p_version;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_workflow_workspace(p_team uuid,p_project uuid,p_round uuid,p_actor uuid,p_session uuid) RETURNS jsonb LANGUAGE plpgsql STABLE SECURITY DEFINER AS $$
DECLARE r bonus_rounds%ROWTYPE; v bonus_round_versions%ROWTYPE; manager boolean; edit boolean; budget boolean; review boolean;
BEGIN
 IF NOT bonus_live(p_team,p_project,p_actor,p_session) THEN RAISE EXCEPTION 'resource not found' USING ERRCODE='P0002'; END IF;
 manager:=bonus_manager(p_team,p_project,p_actor);
 IF p_round IS NULL THEN
 IF NOT manager AND NOT EXISTS(SELECT 1 FROM bonus_rounds br WHERE br.team_id=p_team AND br.project_id=p_project AND bonus_has_role(br.id,p_actor,NULL)) THEN RAISE EXCEPTION 'resource not found' USING ERRCODE='P0002'; END IF;
 RETURN jsonb_build_object('can_create',manager,
 'members',COALESCE((SELECT jsonb_agg(jsonb_build_object('user_id',pm.user_id,'display_name',u.display_name) ORDER BY u.display_name,pm.user_id)
 FROM project_members pm JOIN users u ON u.id=pm.user_id WHERE pm.team_id=p_team AND pm.project_id=p_project AND contribution_member(p_team,p_project,pm.user_id)),'[]'::jsonb),
 'sources',COALESCE((SELECT jsonb_agg(jsonb_build_object('confirmation_id',f.id,'case_version_id',f.case_version_id,'task_title',t.title,
 'shares',(SELECT jsonb_agg(jsonb_build_object('user_id',cs.user_id,'display_name',u.display_name,'share_bps',cs.share_bps) ORDER BY cs.user_id)
 FROM contribution_shares cs JOIN users u ON u.id=cs.user_id WHERE cs.case_version_id=f.case_version_id)) ORDER BY f.confirmed_at,f.id)
 FROM contribution_confirmations f JOIN tasks t ON t.id=f.task_id WHERE f.team_id=p_team AND f.project_id=p_project AND bonus_source_current(f.id)),'[]'::jsonb));
 END IF;
 SELECT * INTO r FROM bonus_rounds WHERE team_id=p_team AND project_id=p_project AND id=p_round;
 IF r.id IS NULL OR NOT bonus_has_role(r.id,p_actor,NULL) THEN RAISE EXCEPTION 'resource not found' USING ERRCODE='P0002'; END IF;
 SELECT * INTO v FROM bonus_round_versions WHERE id=r.latest_version_id;
 edit:=bonus_can_edit(r.id,p_actor); budget:=bonus_has_role(r.id,p_actor,'editor') AND bonus_has_role(r.id,p_actor,'budget'); review:=bonus_has_role(r.id,p_actor,'confirmer');
 RETURN jsonb_build_object('round',jsonb_build_object('id',r.id,'project_id',r.project_id,'funding_key',r.funding_key,'funding_label',r.funding_label,
 'funding_source',r.funding_source,'revision',r.revision,'created_at',r.created_at,'amounts_visible',true,
 'latest_version',bonus_metadata_version(r.latest_version_id,false),'effective_confirmed_version',bonus_metadata_version(r.effective_confirmed_version_id,true)),
 'version',bonus_plan_json(v.id),
 'permissions',jsonb_build_object('edit',COALESCE(edit AND v.status='draft',false),'submit',COALESCE(edit AND v.status='draft' AND EXISTS(SELECT 1 FROM bonus_round_sources WHERE version_id=v.id),false),
 'return',COALESCE(review AND v.status='pending_review',false),'confirm',COALESCE(review AND v.status='pending_review' AND bonus_independent(v.id,p_actor),false),
 'publish',COALESCE(budget AND v.status='confirmed' AND v.published_at IS NULL,false),
 'revise',COALESCE(edit AND v.status='confirmed' AND NOT EXISTS(SELECT 1 FROM bonus_payments WHERE round_id=r.id),false),
 'record_payment',COALESCE(budget AND v.status='confirmed' AND v.published_at IS NOT NULL AND r.effective_confirmed_version_id=v.id,false)),
 'grants',COALESCE((SELECT jsonb_agg(jsonb_build_object('subject_id',g.subject_id,'display_name',u.display_name,'role_code',g.role_code) ORDER BY g.subject_id,g.role_code)
 FROM bonus_role_grants g JOIN users u ON u.id=g.subject_id JOIN project_members pm ON pm.team_id=g.team_id AND pm.project_id=g.project_id AND pm.user_id=g.subject_id
 WHERE g.round_id=r.id AND g.status='active' AND g.grant_mode='internal' AND pm.authorization_version=g.member_authorization_version AND contribution_member(p_team,p_project,g.subject_id)),'[]'::jsonb),
 'events',COALESCE((SELECT jsonb_agg(jsonb_build_object('event_type',e.event_type,'reason',e.reason,'actor_id',e.actor_id,'created_at',e.created_at) ORDER BY e.created_at,e.id) FROM bonus_events e WHERE e.round_id=r.id),'[]'::jsonb),
 'payments',COALESCE((SELECT jsonb_agg(jsonb_build_object('user_id',p.user_id,'amount_minor',p.amount_minor,'reference',p.reference,'recorded_at',p.recorded_at) ORDER BY p.recorded_at,p.id) FROM bonus_payments p WHERE p.round_id=r.id),'[]'::jsonb));
END;
$$;
-- +goose StatementEnd

-- Team removal does not necessarily alter project membership epochs. Revoke
-- bonus grants on departure so rejoining a team never resurrects authority.
-- A trigger-only definer is needed because membership writers cannot DML bonus
-- tables. Reject rebinding even if EXECUTE is accidentally granted: a caller
-- can own a temporary lookalike table without owning the real team_members.
-- Embed the migration schema in the regclass, not the caller's search_path.
-- +goose StatementBegin
DO $migration$
BEGIN
 EXECUTE format($function$
CREATE FUNCTION bonus_membership_invalidate() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER AS $body$
DECLARE rid uuid;
BEGIN
 IF TG_RELID<>%L::regclass OR TG_OP<>'UPDATE' OR TG_LEVEL<>'ROW' OR TG_WHEN<>'AFTER' THEN
 RAISE EXCEPTION 'bonus membership invalidation requires the real team membership trigger' USING ERRCODE='42501'; END IF;
 IF OLD.status='active' AND NEW.status<>'active' THEN
 FOR rid IN SELECT DISTINCT round_id FROM bonus_role_grants WHERE team_id=NEW.team_id AND subject_id=NEW.user_id AND status='active' LOOP
 UPDATE bonus_role_grants SET status='revoked' WHERE round_id=rid AND subject_id=NEW.user_id AND status='active';
 UPDATE bonus_rounds SET revision=revision+1 WHERE id=rid AND latest_version_id IS NOT NULL;
 INSERT INTO bonus_events(team_id,project_id,round_id,version_id,actor_id,event_type,reason,details,request_id)
 SELECT team_id,project_id,id,latest_version_id,NEW.user_id,'membership_invalidated','Automatic grant invalidation after team departure',jsonb_build_object('subject_id',NEW.user_id),gen_random_uuid()::text FROM bonus_rounds WHERE id=rid;
 END LOOP;
 END IF;
 RETURN NEW;
END;
$body$;
$function$, format('%I.team_members',current_schema()));
END;
$migration$;
-- +goose StatementEnd
CREATE TRIGGER bonus_membership_departure AFTER UPDATE OF status ON team_members FOR EACH ROW EXECUTE FUNCTION bonus_membership_invalidate();

-- Pin every bonus function, including v10 trigger helpers, to the migration's
-- actual schema. pg_temp is last, never implicitly searched first. Revoke PUBLIC
-- EXECUTE on v10 helpers too so the reviewed v11 role has only two entry points.
-- Only authenticated entry points and the trigger-only invalidator are definers.
-- +goose StatementBegin
DO $$ DECLARE f record; BEGIN
 FOR f IN SELECT oid::regprocedure AS signature FROM pg_proc WHERE pronamespace=current_schema()::regnamespace
 AND left(proname,6)='bonus_' LOOP
 EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,current_schema());
 EXECUTE format('REVOKE ALL ON FUNCTION %s FROM PUBLIC',f.signature);
 END LOOP;
END $$;
-- +goose StatementEnd
REVOKE ALL ON bonus_payments,bonus_commands FROM PUBLIC;

-- Run explicitly as the migration owner with the reviewed application role.
-- SECURITY INVOKER: granting EXECUTE on this helper cannot elevate an app role.
-- +goose StatementBegin
CREATE FUNCTION bonus_configure_application_role(p_role name) RETURNS void LANGUAGE plpgsql AS $$
DECLARE s name:=current_schema(); t text; f record;
BEGIN
 FOREACH t IN ARRAY ARRAY['bonus_rounds','bonus_round_versions','bonus_round_sources','bonus_eligibility_decisions','bonus_allocation_lines','bonus_adjustments','bonus_role_grants','bonus_confirmations','bonus_events','bonus_payments','bonus_commands'] LOOP
 EXECUTE format('REVOKE INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER ON %I.%I FROM %I',s,t,p_role);
 EXECUTE format('GRANT SELECT ON %I.%I TO %I',s,t,p_role);
 END LOOP;
 EXECUTE format('GRANT SELECT ON %I.bonus_member_totals TO %I',s,p_role);
 FOR f IN SELECT oid::regprocedure AS signature FROM pg_proc WHERE pronamespace=s::regnamespace AND left(proname,6)='bonus_' LOOP
 EXECUTE format('REVOKE ALL ON FUNCTION %s FROM %I',f.signature,p_role);
 END LOOP;
 EXECUTE format('GRANT EXECUTE ON FUNCTION %I.bonus_workflow_command(uuid,uuid,uuid,uuid,uuid,text,jsonb), %I.bonus_workflow_workspace(uuid,uuid,uuid,uuid,uuid) TO %I',s,s,p_role);
END;
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION bonus_configure_application_role(name) FROM PUBLIC;

-- +goose Down
-- Downgrade would reintroduce M2 semantics for already authorized facts.
-- Refuse rather than silently erase financial or governance history.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'bonus workflow downgrade requires an explicitly reviewed forward recovery migration'; END $$;
-- +goose StatementEnd
