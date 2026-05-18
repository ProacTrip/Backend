-- +migrate Up
-- Add dashboard permissions that were defined in code but never seeded

INSERT INTO permissions(resource, action, description) VALUES
    ('feature_limits', 'read', 'Ver límites de features'),
    ('feature_limits', 'write', 'Gestionar límites de features'),
    ('permissions', 'read', 'Ver permisos RBAC'),
    ('permissions', 'write', 'Gestionar permisos RBAC')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign all new permissions to admin role
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id 
FROM roles r 
CROSS JOIN permissions p 
WHERE r.name = 'admin' 
  AND p.resource IN ('feature_limits', 'permissions')
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- +migrate Down
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id 
  AND rp.permission_id = p.id 
  AND r.name = 'admin' 
  AND p.resource IN ('feature_limits', 'permissions');

DELETE FROM permissions WHERE resource IN ('feature_limits', 'permissions');
