-- +goose Up
-- Phase06 M2: schema-only foundation for independent post-archive CNY bonus rounds.
-- IMPORTANT: M2 does NOT authorize confirmations, payments, active bonus grants,
-- or money algorithm execution. The M3 policy/authorization migration must
-- explicitly replace the conservative M2 write guards before those operations.
-- Never change the historical migrations 00001..00008.

CREATE TABLE bonus_rounds (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 funding_key text NOT NULL CHECK (
  length(funding_key) BETWEEN 1 AND 120
  AND funding_key=lower(btrim(funding_key))
  AND funding_key ~ '^[a-z0-9][a-z0-9_-]{0,119}$'
 ),
 funding_label text NOT NULL CHECK(length(btrim(funding_label)) BETWEEN 1 AND 200),
 funding_source text NOT NULL CHECK(length(btrim(funding_source)) BETWEEN 1 AND 400),
 created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 latest_version_id uuid,
 effective_confirmed_version_id uuid,
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 UNIQUE(team_id,project_id,id),
 UNIQUE(team_id,project_id,funding_key),
 FOREIGN KEY(team_id,project_id) REFERENCES projects(team_id,id) ON DELETE RESTRICT
);
CREATE INDEX bonus_rounds_project_idx ON bonus_rounds(team_id,project_id,created_at,id);

CREATE TABLE bonus_round_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 round_id uuid NOT NULL,
 version integer NOT NULL CHECK(version>0),
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','pending_review','confirmed','superseded','overturned','abandoned')),
 previous_version_id uuid,
 total_budget_minor bigint NOT NULL DEFAULT 0 CHECK(total_budget_minor BETWEEN 0 AND 900000000000000),
 plan_unallocated_minor bigint NOT NULL DEFAULT 0 CHECK(plan_unallocated_minor>=0 AND plan_unallocated_minor<=total_budget_minor),
 currency_code text NOT NULL DEFAULT 'CNY' CHECK(currency_code='CNY'),
 currency_minor_unit smallint NOT NULL DEFAULT 2 CHECK(currency_minor_unit=2),
 agreement_snapshot text NOT NULL DEFAULT '' CHECK(length(agreement_snapshot)<=8000),
 proposed_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 rationale text NOT NULL CHECK(length(btrim(rationale)) BETWEEN 1 AND 4000),
 client_request_id uuid NOT NULL,
 submitted_at timestamptz,
 submitted_digest text CHECK(submitted_digest ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,round_id,id),
 UNIQUE(round_id,version),
 UNIQUE(round_id,client_request_id),
 FOREIGN KEY(team_id,project_id,round_id) REFERENCES bonus_rounds(team_id,project_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,round_id,previous_version_id) REFERENCES bonus_round_versions(team_id,project_id,round_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((status='draft' AND submitted_at IS NULL AND submitted_digest IS NULL) OR status<>'draft')
);
CREATE UNIQUE INDEX bonus_one_working_version ON bonus_round_versions(team_id,project_id,round_id) WHERE status IN ('draft','pending_review');
CREATE INDEX bonus_round_versions_history_idx ON bonus_round_versions(team_id,project_id,round_id,version DESC);
ALTER TABLE bonus_rounds ADD CONSTRAINT bonus_rounds_latest_fk
 FOREIGN KEY(team_id,project_id,id,latest_version_id) REFERENCES bonus_round_versions(team_id,project_id,round_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE bonus_rounds ADD CONSTRAINT bonus_rounds_effective_fk
 FOREIGN KEY(team_id,project_id,id,effective_confirmed_version_id) REFERENCES bonus_round_versions(team_id,project_id,round_id,id) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE bonus_round_sources (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 round_id uuid NOT NULL,
 version_id uuid NOT NULL,
 task_id uuid NOT NULL,
 case_id uuid NOT NULL,
 case_version_id uuid NOT NULL,
 confirmation_id uuid NOT NULL,
 confirmation_digest text NOT NULL CHECK(confirmation_digest ~ '^[0-9a-f]{64}$'),
 coverage_decision text NOT NULL CHECK(coverage_decision IN ('included','excluded')),
 source_budget_minor bigint NOT NULL DEFAULT 0 CHECK(source_budget_minor BETWEEN 0 AND 900000000000000),
 source_unallocated_minor bigint NOT NULL DEFAULT 0 CHECK(source_unallocated_minor>=0 AND source_unallocated_minor<=source_budget_minor),
 shares_snapshot jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(shares_snapshot)='array'),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,round_id,version_id,id,case_version_id),
 UNIQUE(version_id,confirmation_id),
 FOREIGN KEY(team_id,project_id,round_id,version_id) REFERENCES bonus_round_versions(team_id,project_id,round_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id,confirmation_id) REFERENCES contribution_confirmations(team_id,project_id,task_id,case_id,case_version_id,id) ON DELETE RESTRICT,
 CHECK(coverage_decision='included' OR (source_budget_minor=0 AND source_unallocated_minor=0))
);
-- Only a single confirmation per bonus version, NOT a global unique confirmation.
-- The same accepted contribution may fund independently identified bonus rounds.
CREATE INDEX bonus_round_sources_confirmation_idx ON bonus_round_sources(team_id,project_id,confirmation_id);
CREATE INDEX bonus_round_sources_version_idx ON bonus_round_sources(team_id,project_id,round_id,version_id);

CREATE TABLE bonus_eligibility_decisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 round_id uuid NOT NULL,
 version_id uuid NOT NULL,
 user_id uuid NOT NULL,
 decision text NOT NULL CHECK(decision IN ('eligible','ineligible')),
 member_authorization_version integer NOT NULL CHECK(member_authorization_version>0),
 decided_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000),
 decided_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(version_id,user_id),
 UNIQUE(team_id,project_id,round_id,version_id,user_id,id),
 FOREIGN KEY(team_id,project_id,round_id,version_id) REFERENCES bonus_round_versions(team_id,project_id,round_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,user_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 CHECK(decided_by<>user_id)
);
CREATE INDEX bonus_eligibility_member_idx ON bonus_eligibility_decisions(team_id,project_id,user_id,version_id);

CREATE TABLE bonus_allocation_lines (
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 round_id uuid NOT NULL,
 version_id uuid NOT NULL,
 source_id uuid NOT NULL,
 case_version_id uuid NOT NULL,
 user_id uuid NOT NULL,
 eligibility_decision_id uuid NOT NULL,
 share_bps integer NOT NULL CHECK(share_bps BETWEEN 1 AND 10000),
 raw_rounded_minor bigint NOT NULL CHECK(raw_rounded_minor BETWEEN 0 AND 900000000000000),
 suggested_minor bigint NOT NULL CHECK(suggested_minor BETWEEN 0 AND 900000000000000),
 final_minor bigint NOT NULL CHECK(final_minor BETWEEN 0 AND 900000000000000),
 remainder_rank integer NOT NULL DEFAULT 0 CHECK(remainder_rank>=0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(team_id,project_id,round_id,version_id,source_id,user_id),
 FOREIGN KEY(team_id,project_id,round_id,version_id,source_id,case_version_id) REFERENCES bonus_round_sources(team_id,project_id,round_id,version_id,id,case_version_id) ON DELETE RESTRICT,
 FOREIGN KEY(case_version_id,user_id) REFERENCES contribution_shares(case_version_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,round_id,version_id,user_id,eligibility_decision_id) REFERENCES bonus_eligibility_decisions(team_id,project_id,round_id,version_id,user_id,id) ON DELETE RESTRICT
);
CREATE INDEX bonus_allocation_member_idx ON bonus_allocation_lines(team_id,project_id,user_id,version_id);
-- This view is the ONLY member-level aggregate. There is no independently writable
-- bonus member total that could diverge from per-source ledger rows.
CREATE VIEW bonus_member_totals AS
 SELECT team_id,project_id,round_id,version_id,user_id,sum(final_minor) AS total_minor
 FROM bonus_allocation_lines
 GROUP BY team_id,project_id,round_id,version_id,user_id;

CREATE TABLE bonus_adjustments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 round_id uuid NOT NULL,
 version_id uuid NOT NULL,
 source_id uuid NOT NULL,
 user_id uuid NOT NULL,
 old_minor bigint NOT NULL CHECK(old_minor BETWEEN 0 AND 900000000000000),
 new_minor bigint NOT NULL CHECK(new_minor BETWEEN 0 AND 900000000000000),
 delta_minor bigint NOT NULL CHECK(delta_minor=new_minor-old_minor),
 old_source_unallocated_minor bigint NOT NULL CHECK(old_source_unallocated_minor>=0),
 new_source_unallocated_minor bigint NOT NULL CHECK(new_source_unallocated_minor>=0),
 actor_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(version_id,request_id),
 FOREIGN KEY(team_id,project_id,round_id,version_id,source_id,user_id)
  REFERENCES bonus_allocation_lines(team_id,project_id,round_id,version_id,source_id,user_id) ON DELETE RESTRICT
);
CREATE INDEX bonus_adjustments_source_idx ON bonus_adjustments(team_id,project_id,round_id,version_id,source_id,created_at);

CREATE TABLE bonus_role_grants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 round_id uuid NOT NULL,
 subject_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 role_code text NOT NULL CHECK(role_code IN ('editor','budget','eligibility','confirmer','auditor')),
 grant_mode text NOT NULL CHECK(grant_mode IN ('internal','external')),
 status text NOT NULL DEFAULT 'proposed' CHECK(status IN ('proposed','active','revoked')),
 member_authorization_version integer,
 identity_evidence_ref text NOT NULL DEFAULT '',
 expires_at timestamptz,
 granted_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(round_id,request_id),
 UNIQUE(team_id,project_id,round_id,id),
 FOREIGN KEY(team_id,project_id,round_id) REFERENCES bonus_rounds(team_id,project_id,id) ON DELETE RESTRICT,
 CHECK(subject_id<>granted_by),
 CHECK((grant_mode='internal' AND member_authorization_version IS NOT NULL AND member_authorization_version>0 AND expires_at IS NULL)
   OR (grant_mode='external' AND member_authorization_version IS NULL AND expires_at IS NOT NULL AND length(btrim(identity_evidence_ref))>0))
);
CREATE INDEX bonus_role_grants_subject_idx ON bonus_role_grants(team_id,project_id,round_id,subject_id,role_code);

CREATE TABLE bonus_confirmations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 round_id uuid NOT NULL,
 version_id uuid NOT NULL UNIQUE,
 grant_id uuid NOT NULL,
 confirmed_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 version_digest text NOT NULL CHECK(version_digest ~ '^[0-9a-f]{64}$'),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 confirmed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,round_id,version_id,id),
 UNIQUE(round_id,request_id),
 FOREIGN KEY(team_id,project_id,round_id,version_id) REFERENCES bonus_round_versions(team_id,project_id,round_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,round_id,grant_id) REFERENCES bonus_role_grants(team_id,project_id,round_id,id) ON DELETE RESTRICT
);

CREATE TABLE bonus_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 round_id uuid NOT NULL,
 version_id uuid,
 actor_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 event_type text NOT NULL CHECK(length(btrim(event_type)) BETWEEN 1 AND 120),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000),
 details jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(details)='object'),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(team_id,project_id,round_id) REFERENCES bonus_rounds(team_id,project_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,round_id,version_id) REFERENCES bonus_round_versions(team_id,project_id,round_id,id) ON DELETE RESTRICT
);
CREATE INDEX bonus_events_round_idx ON bonus_events(team_id,project_id,round_id,created_at,id);

-- +goose StatementBegin
CREATE FUNCTION bonus_m2_immutable_guard() RETURNS trigger AS $$
BEGIN
 RAISE EXCEPTION 'M2 bonus foundation is append-only; changes require an authorized later migration'
  USING ERRCODE='23514',CONSTRAINT='bonus_m2_immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_m2_source_guard() RETURNS trigger AS $$
DECLARE ok boolean;
BEGIN
 -- Use the existing team lock order, but do NOT require projects.status='active'.
 PERFORM 1 FROM teams WHERE id=NEW.team_id FOR UPDATE;
 SELECT (
  c.effective_confirmed_version_id=NEW.case_version_id
  AND c.case_workflow_status='confirmed'
  AND v.status='confirmed'
  AND f.version_digest=NEW.confirmation_digest
  AND NOT contribution_open_disputes(c.id)
 ) INTO ok
 FROM contribution_cases c
 JOIN contribution_case_versions v ON v.id=NEW.case_version_id AND v.case_id=c.id
 JOIN contribution_confirmations f ON f.id=NEW.confirmation_id AND f.case_version_id=v.id
 WHERE c.team_id=NEW.team_id AND c.project_id=NEW.project_id
   AND c.task_id=NEW.task_id AND c.id=NEW.case_id;
 IF COALESCE(ok,false) IS NOT TRUE THEN
  RAISE EXCEPTION 'bonus source requires current effective confirmed contribution evidence'
   USING ERRCODE='23514',CONSTRAINT='bonus_source_not_effective';
 END IF;
 IF NOT EXISTS(
  SELECT 1 FROM bonus_round_versions v
  WHERE v.team_id=NEW.team_id AND v.project_id=NEW.project_id
    AND v.round_id=NEW.round_id AND v.id=NEW.version_id AND v.status='draft'
 ) THEN
  RAISE EXCEPTION 'bonus source must belong to a draft version'
   USING ERRCODE='23514',CONSTRAINT='bonus_source_not_draft';
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_m2_line_guard() RETURNS trigger AS $$
DECLARE actual_bps integer; eligibility text; coverage text;
BEGIN
 SELECT share_bps INTO actual_bps FROM contribution_shares
 WHERE case_version_id=NEW.case_version_id AND user_id=NEW.user_id;
 SELECT coverage_decision INTO coverage FROM bonus_round_sources
 WHERE team_id=NEW.team_id AND project_id=NEW.project_id
 AND round_id=NEW.round_id AND version_id=NEW.version_id AND id=NEW.source_id;
 SELECT decision INTO eligibility FROM bonus_eligibility_decisions
 WHERE team_id=NEW.team_id AND project_id=NEW.project_id
 AND round_id=NEW.round_id AND version_id=NEW.version_id
 AND user_id=NEW.user_id AND id=NEW.eligibility_decision_id;
 IF actual_bps IS NULL OR actual_bps<>NEW.share_bps OR coverage IS DISTINCT FROM 'included'
 OR eligibility IS NULL THEN
  RAISE EXCEPTION 'bonus line must match an included confirmed share and explicit eligibility'
   USING ERRCODE='23514',CONSTRAINT='bonus_line_invalid_evidence';
 END IF;
 IF (eligibility='ineligible' AND (NEW.suggested_minor<>0 OR NEW.final_minor<>0))
 OR (eligibility='eligible' AND NEW.suggested_minor<>NEW.raw_rounded_minor) THEN
  RAISE EXCEPTION 'bonus line cannot silently reallocate ineligible shares'
   USING ERRCODE='23514',CONSTRAINT='bonus_line_eligibility_mismatch';
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION bonus_m2_finalization_guard() RETURNS trigger AS $$
BEGIN
 IF TG_TABLE_NAME='bonus_round_versions' THEN
  IF NEW.status<>'draft' THEN
   RAISE EXCEPTION 'M2 does not enable bonus submissions or confirmations'
    USING ERRCODE='23514',CONSTRAINT='bonus_m2_not_finalized';
  END IF;
 ELSIF TG_TABLE_NAME='bonus_role_grants' THEN
  IF NEW.status<>'proposed' THEN
   RAISE EXCEPTION 'M2 cannot activate bonus grants'
    USING ERRCODE='23514',CONSTRAINT='bonus_m2_grants_disabled';
  END IF;
 ELSE
  RAISE EXCEPTION 'M2 cannot create bonus confirmations'
   USING ERRCODE='23514',CONSTRAINT='bonus_m2_confirmation_disabled';
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER bonus_m2_version_finalization BEFORE INSERT ON bonus_round_versions
 FOR EACH ROW EXECUTE FUNCTION bonus_m2_finalization_guard();
CREATE TRIGGER bonus_m2_grant_activation BEFORE INSERT ON bonus_role_grants
 FOR EACH ROW EXECUTE FUNCTION bonus_m2_finalization_guard();
CREATE TRIGGER bonus_m2_confirmation_disabled BEFORE INSERT ON bonus_confirmations
 FOR EACH ROW EXECUTE FUNCTION bonus_m2_finalization_guard();
CREATE TRIGGER bonus_m2_source_valid BEFORE INSERT ON bonus_round_sources
 FOR EACH ROW EXECUTE FUNCTION bonus_m2_source_guard();
CREATE TRIGGER bonus_m2_line_valid BEFORE INSERT ON bonus_allocation_lines
 FOR EACH ROW EXECUTE FUNCTION bonus_m2_line_guard();

-- The Phase06 workflow/API is not implemented yet; the M2 foundation refuses
-- updates/deletes rather than suggesting it already provides audited transitions.
CREATE TRIGGER bonus_m2_lock BEFORE UPDATE OR DELETE ON bonus_rounds FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();
CREATE TRIGGER bonus_m2_lock BEFORE UPDATE OR DELETE ON bonus_round_versions FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();
CREATE TRIGGER bonus_m2_lock BEFORE UPDATE OR DELETE ON bonus_round_sources FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();
CREATE TRIGGER bonus_m2_lock BEFORE UPDATE OR DELETE ON bonus_eligibility_decisions FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();
CREATE TRIGGER bonus_m2_lock BEFORE UPDATE OR DELETE ON bonus_allocation_lines FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();
CREATE TRIGGER bonus_m2_lock BEFORE UPDATE OR DELETE ON bonus_adjustments FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();
CREATE TRIGGER bonus_m2_lock BEFORE UPDATE OR DELETE ON bonus_role_grants FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();
CREATE TRIGGER bonus_m2_lock BEFORE UPDATE OR DELETE ON bonus_confirmations FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();
CREATE TRIGGER bonus_m2_lock BEFORE UPDATE OR DELETE ON bonus_events FOR EACH ROW EXECUTE FUNCTION bonus_m2_immutable_guard();

REVOKE ALL ON bonus_rounds,bonus_round_versions,bonus_round_sources,
 bonus_eligibility_decisions,bonus_allocation_lines,bonus_adjustments,
 bonus_role_grants,bonus_confirmations,bonus_events FROM PUBLIC;
REVOKE ALL ON bonus_member_totals FROM PUBLIC;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM bonus_rounds) THEN
  RAISE EXCEPTION 'cannot roll back bonus foundation with funding round evidence'
   USING ERRCODE='23514',CONSTRAINT='bonus_rollback_has_evidence';
 END IF;
END $$;
-- +goose StatementEnd
DROP VIEW bonus_member_totals;
DROP TABLE bonus_events,bonus_confirmations,bonus_adjustments,
 bonus_allocation_lines,bonus_eligibility_decisions,bonus_round_sources,
 bonus_role_grants,bonus_round_versions,bonus_rounds;
DROP FUNCTION bonus_m2_finalization_guard();
DROP FUNCTION bonus_m2_line_guard();
DROP FUNCTION bonus_m2_source_guard();
DROP FUNCTION bonus_m2_immutable_guard();
