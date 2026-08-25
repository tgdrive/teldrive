INSERT INTO users (user_id, display_name, username, role, disabled_at)
VALUES
  (1001, 'Playwright Owner', 'owner', 'owner', NULL),
  (1002, 'Playwright Admin', 'admin', 'admin', NULL),
  (1003, 'Alice Example', 'alice', 'user', NULL),
  (1004, 'Bob Example', 'bob', 'user', NULL),
  (1005, 'Charlie Example', 'charlie', 'user', NULL),
  (1006, 'Disabled Example', 'disabled', 'user', now())
ON CONFLICT (user_id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    username = EXCLUDED.username,
    role = EXCLUDED.role,
    disabled_at = EXCLUDED.disabled_at;

INSERT INTO files (id, user_id, parent_id, name, normalized_name, kind, mime_type, size, status, mod_time, created_at, updated_at)
VALUES
  ('70000000-0000-4000-8000-000000000001', 1003, NULL, 'Read shared', 'Read shared', 'folder', NULL, NULL, 'active', now(), now(), now()),
  ('70000000-0000-4000-8000-000000000002', 1003, '70000000-0000-4000-8000-000000000001', 'readme.txt', 'readme.txt', 'file', 'text/plain', 12, 'active', now(), now(), now()),
  ('70000000-0000-4000-8000-000000000003', 1003, NULL, 'Edit shared', 'Edit shared', 'folder', NULL, NULL, 'active', now(), now(), now()),
  ('70000000-0000-4000-8000-000000000004', 1003, '70000000-0000-4000-8000-000000000003', 'editable.txt', 'editable.txt', 'file', 'text/plain', 14, 'active', now(), now(), now()),
  ('70000000-0000-4000-8000-000000000005', 1003, NULL, 'Private Alice', 'Private Alice', 'folder', NULL, NULL, 'active', now(), now(), now()),
  ('70000000-0000-4000-8000-000000000006', 1003, NULL, 'Expired shared', 'Expired shared', 'folder', NULL, NULL, 'active', now(), now(), now()),
  ('70000000-0000-4000-8000-000000000007', 1003, NULL, 'Revoked shared', 'Revoked shared', 'folder', NULL, NULL, 'active', now(), now(), now())
ON CONFLICT (id) DO NOTHING;

INSERT INTO file_access_grants (id, file_id, owner_id, grantee_id, permission, expires_at, revoked_at)
VALUES
  ('71000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000001', 1003, 1004, 'read', NULL, NULL),
  ('71000000-0000-4000-8000-000000000002', '70000000-0000-4000-8000-000000000003', 1003, 1004, 'edit', NULL, NULL),
  ('71000000-0000-4000-8000-000000000003', '70000000-0000-4000-8000-000000000006', 1003, 1004, 'read', now() - interval '1 hour', NULL),
  ('71000000-0000-4000-8000-000000000004', '70000000-0000-4000-8000-000000000007', 1003, 1004, 'read', NULL, now())
ON CONFLICT (id) DO NOTHING;
