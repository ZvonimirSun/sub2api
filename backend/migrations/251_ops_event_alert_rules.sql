-- Add an event-driven alert rule for model fingerprint changes.
-- Existing metric alert rows and schema remain unchanged.
INSERT INTO ops_alert_rules (
    name,
    description,
    enabled,
    severity,
    metric_type,
    operator,
    threshold,
    window_minutes,
    sustained_minutes,
    cooldown_minutes,
    notify_email,
    created_at,
    updated_at
) VALUES (
    '模型指纹发生变化',
    '模型指纹探测成功后，新指纹与上次有效结果不同时立即触发。',
    true,
    'P0',
    'event:modeltrace_fingerprint',
    '==',
    0,
    1,
    1,
    0,
    true,
    NOW(),
    NOW()
) ON CONFLICT (name) DO NOTHING;
