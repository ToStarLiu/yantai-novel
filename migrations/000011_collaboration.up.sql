CREATE TABLE wb_users (
 id text PRIMARY KEY, username text NOT NULL UNIQUE, display_name text NOT NULL,
 password_hash text NOT NULL, disabled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE wb_sessions ADD COLUMN user_id text REFERENCES wb_users(id) ON DELETE CASCADE;
ALTER TABLE wb_sessions ADD COLUMN is_admin boolean NOT NULL DEFAULT true;
ALTER TABLE wb_sessions ADD COLUMN username text NOT NULL DEFAULT '';
CREATE TABLE wb_project_members (
 project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 user_id text NOT NULL REFERENCES wb_users(id) ON DELETE CASCADE,
 role text NOT NULL CHECK(role IN ('owner','editor','reviewer','viewer')),
 joined_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(project_id,user_id)
);
CREATE INDEX wb_project_members_user ON wb_project_members(user_id,project_id);
CREATE TABLE wb_invitations (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 role text NOT NULL CHECK(role IN ('owner','editor','reviewer','viewer')),
 token_hash text NOT NULL UNIQUE, created_by text NOT NULL DEFAULT '',
 expires_at timestamptz NOT NULL, accepted_by text NOT NULL DEFAULT '', revoked boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE wb_presence (
 project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 user_id text NOT NULL, username text NOT NULL, display_name text NOT NULL,
 chapter_id text NOT NULL DEFAULT '', revision_id text NOT NULL DEFAULT '', cursor_position integer,
 seen_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(project_id,user_id)
);
CREATE TABLE wb_collab_events (
 id bigserial PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 kind text NOT NULL, object_id text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_collab_events_project ON wb_collab_events(project_id,id);
CREATE FUNCTION wb_record_collab_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE row_data jsonb; pid text; oid text;
BEGIN
 IF TG_OP = 'DELETE' THEN row_data := to_jsonb(OLD); ELSE row_data := to_jsonb(NEW); END IF;
 pid := row_data->>'project_id'; oid := COALESCE(row_data->>'id',row_data->>'user_id','main');
 IF TG_TABLE_NAME = 'wb_projects' THEN pid := row_data->>'id'; END IF;
 IF TG_TABLE_NAME = 'wb_revisions' THEN SELECT project_id INTO pid FROM wb_chapters WHERE id=row_data->>'chapter_id'; END IF;
 IF TG_TABLE_NAME = 'wb_studio_documents' THEN oid := (row_data->>'kind') || ':' || (row_data->>'id'); END IF;
 IF pid IS NOT NULL AND EXISTS(SELECT 1 FROM wb_projects WHERE id=pid) THEN
   INSERT INTO wb_collab_events(project_id,kind,object_id) VALUES(pid,TG_TABLE_NAME || ':' || lower(TG_OP),oid);
 END IF;
 IF TG_OP = 'DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END $$;
CREATE TRIGGER wb_project_changes AFTER UPDATE ON wb_projects FOR EACH ROW EXECUTE FUNCTION wb_record_collab_change();
CREATE TRIGGER wb_chapter_changes AFTER INSERT OR UPDATE OR DELETE ON wb_chapters FOR EACH ROW EXECUTE FUNCTION wb_record_collab_change();
CREATE TRIGGER wb_revision_changes AFTER INSERT ON wb_revisions FOR EACH ROW EXECUTE FUNCTION wb_record_collab_change();
CREATE TRIGGER wb_setting_changes AFTER INSERT OR UPDATE OR DELETE ON wb_settings FOR EACH ROW EXECUTE FUNCTION wb_record_collab_change();
CREATE TRIGGER wb_studio_changes AFTER INSERT OR UPDATE OR DELETE ON wb_studio_documents FOR EACH ROW EXECUTE FUNCTION wb_record_collab_change();
CREATE TRIGGER wb_member_changes AFTER INSERT OR UPDATE OR DELETE ON wb_project_members FOR EACH ROW EXECUTE FUNCTION wb_record_collab_change();
