-- 清理 k6 生成的 load-% 测试账号及其关联数据。
DELETE ce
FROM click_events ce
INNER JOIN workspaces w ON w.id = ce.workspace_id
INNER JOIN users u ON u.id = w.owner_user_id
WHERE u.email LIKE 'load-%@example.com';

DELETE i
FROM idempotency_keys i
INNER JOIN workspaces w ON w.id = i.workspace_id
INNER JOIN users u ON u.id = w.owner_user_id
WHERE u.email LIKE 'load-%@example.com';

DELETE l
FROM links l
INNER JOIN workspaces w ON w.id = l.workspace_id
INNER JOIN users u ON u.id = w.owner_user_id
WHERE u.email LIKE 'load-%@example.com';

DELETE d
FROM domains d
INNER JOIN workspaces w ON w.id = d.workspace_id
INNER JOIN users u ON u.id = w.owner_user_id
WHERE u.email LIKE 'load-%@example.com';

DELETE wm
FROM workspace_members wm
INNER JOIN users u ON u.id = wm.user_id
WHERE u.email LIKE 'load-%@example.com';

DELETE w
FROM workspaces w
INNER JOIN users u ON u.id = w.owner_user_id
WHERE u.email LIKE 'load-%@example.com';

DELETE FROM users WHERE email LIKE 'load-%@example.com';
