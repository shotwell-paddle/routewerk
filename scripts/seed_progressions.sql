-- Seed data for the Progressions feature.
-- Run against staging: fly postgres connect -a routewerk-dev-db -d routewerk_dev < scripts/seed_progressions.sql
--
-- Requires a location to already exist. Finds the first location and uses it.

DO $$
DECLARE
  loc_id UUID;
BEGIN
  -- Grab the first location
  SELECT id INTO loc_id FROM locations LIMIT 1;
  IF loc_id IS NULL THEN
    RAISE EXCEPTION 'No locations found — create a gym first via /setup';
  END IF;

  -- ── Quest Domains ────────────────────────────────────────
  INSERT INTO quest_domains (id, location_id, name, description, color, icon, sort_order) VALUES
    (gen_random_uuid(), loc_id, 'Technique',      'Footwork, body positioning, and movement patterns',       '#2196F3', 'foot',     1),
    (gen_random_uuid(), loc_id, 'Endurance',       'Stamina, pump management, and sustained climbing',        '#4CAF50', 'heart',    2),
    (gen_random_uuid(), loc_id, 'Power',           'Dynamic moves, campus skills, and explosive strength',    '#F44336', 'zap',      3),
    (gen_random_uuid(), loc_id, 'Mental Game',     'Fear management, projecting mindset, and focus',          '#9C27B0', 'brain',    4),
    (gen_random_uuid(), loc_id, 'Route Reading',   'Visualization, sequencing, and beta discovery',           '#FF9800', 'eye',      5),
    (gen_random_uuid(), loc_id, 'Flexibility',     'Hip mobility, high steps, and body tension',              '#00BCD4', 'stretch',  6),
    (gen_random_uuid(), loc_id, 'Crack & Slab',    'Specialized face and crack climbing technique',           '#795548', 'mountain', 7),
    (gen_random_uuid(), loc_id, 'Community',        'Belaying, spotting, coaching, and gym involvement',       '#607D8B', 'users',    8)
  ON CONFLICT DO NOTHING;

  -- ── Badges ───────────────────────────────────────────────
  INSERT INTO badges (id, location_id, name, description, icon, color) VALUES
    (gen_random_uuid(), loc_id, 'First Quest',        'Completed your first quest',                  'star',     '#FFD700'),
    (gen_random_uuid(), loc_id, 'Technique Novice',   'Completed 3 technique quests',                'foot',     '#2196F3'),
    (gen_random_uuid(), loc_id, 'Endurance Machine',  'Completed 3 endurance quests',                'heart',    '#4CAF50'),
    (gen_random_uuid(), loc_id, 'Power Player',       'Completed 3 power quests',                    'zap',      '#F44336'),
    (gen_random_uuid(), loc_id, 'Mind Over Matter',   'Completed a mental game quest',               'brain',    '#9C27B0'),
    (gen_random_uuid(), loc_id, 'Beta Reader',        'Completed 3 route reading quests',            'eye',      '#FF9800'),
    (gen_random_uuid(), loc_id, 'Well Rounded',       'Completed quests in 5+ domains',              'award',    '#E91E63'),
    (gen_random_uuid(), loc_id, 'Community Pillar',   'Completed 3 community quests',                'users',    '#607D8B'),
    (gen_random_uuid(), loc_id, 'Quest Master',       'Completed 10 total quests',                   'trophy',   '#FF5722'),
    (gen_random_uuid(), loc_id, 'Slab Lord',          'Completed the slab specialist quest',         'mountain', '#795548')
  ON CONFLICT DO NOTHING;

  -- ── Quests ───────────────────────────────────────────────
  -- We need the domain IDs, so fetch them by name
  INSERT INTO quests (location_id, domain_id, badge_id, name, description, quest_type, completion_criteria, target_count, suggested_duration_days, skill_level, is_active, sort_order)
  SELECT loc_id, d.id, b.id, q.name, q.description, q.quest_type, q.criteria, q.target, q.days, q.level, true, q.sort
  FROM (VALUES
    -- Technique quests
    ('Technique', 'First Quest',      'Silent Feet',          'Climb 10 routes focusing on silent, precise foot placements. No scraping!',                      'permanent', 'Climb routes with zero foot noise — be deliberate with every placement.',     10, 14, 'beginner',      1),
    ('Technique', 'Technique Novice', 'Flagging Fundamentals','Use flagging technique on 8 different routes. Inside flag, outside flag, and backflag.',          'permanent', 'Log routes where you consciously used flagging to maintain balance.',          8,  14, 'intermediate',  2),
    ('Technique', NULL,               'Twist Lock Tour',      'Complete 5 routes using twist locks and drop knees as your primary technique.',                   'permanent', 'Focus on routes where twist locks and drop knees are the key moves.',          5,  10, 'intermediate',  3),

    -- Endurance quests
    ('Endurance', NULL,               '4x4 Challenge',        'Complete four 4x4 sessions in two weeks. Each 4x4 = 4 boulders, 4 times each, minimal rest.',    'permanent', 'Log each 4x4 session as one entry.',                                          4,  14, 'intermediate',  1),
    ('Endurance', 'Endurance Machine','Pump Clock',           'Climb 15 routes at your onsight grade without falling. Build that base.',                         'permanent', 'Send routes at or just below your limit — clean ascents only.',                15, 21, 'beginner',      2),

    -- Power quests
    ('Power',     NULL,               'Dyno Dozen',           'Stick 12 dynamic moves across any routes or boulders.',                                          'permanent', 'Log each route/boulder where you stuck a significant dynamic move.',           12, 21, 'intermediate',  1),
    ('Power',     'Power Player',     'Campus Board Intro',   'Complete 5 campus board sessions focusing on basic ladder movements.',                            'permanent', 'Log each campus board session.',                                               5,  14, 'advanced',      2),

    -- Mental Game quests
    ('Mental Game','Mind Over Matter','The Project',          'Pick one route above your grade and work it for 2 weeks. Log every attempt.',                     'permanent', 'Log each session working your project — notes required.',                      8,  14, 'intermediate',  1),
    ('Mental Game', NULL,             'Lead Head',            'Lead climb 5 routes where you feel nervous. Practice falling at each bolt.',                      'permanent', 'Log lead routes where you pushed your comfort zone.',                          5,  14, 'intermediate',  2),

    -- Route Reading quests
    ('Route Reading','Beta Reader',   'Flash Attempt Five',   'Attempt to flash 5 routes by reading the beta from the ground before climbing.',                  'permanent', 'Spend 2+ minutes reading each route before your attempt.',                     5,  10, 'beginner',      1),
    ('Route Reading', NULL,           'Sequence Spotter',     'Watch 5 climbers on routes you haven''t tried, predict the crux, then climb.',                    'permanent', 'Log the route and your prediction accuracy in notes.',                         5,  10, 'intermediate',  2),

    -- Flexibility quests
    ('Flexibility', NULL,             'High Step Challenge',  'Complete 8 routes where you use a high step above your waist.',                                   'permanent', 'Look for routes with big moves that reward flexibility.',                      8,  14, 'beginner',      1),

    -- Crack & Slab quests
    ('Crack & Slab','Slab Lord',      'Slab Specialist',      'Send 10 slab routes trusting your feet. Smear with confidence.',                                 'permanent', 'Focus on balance, trust, and tiny footholds.',                                10, 21, 'intermediate',  1),

    -- Community quests
    ('Community', 'Community Pillar', 'Belay Buddy',          'Belay 10 different climbers this month. Spread the stoke.',                                      'permanent', 'Log each belay session — name your partner in notes.',                         10, 30, 'beginner',      1),
    ('Community', NULL,               'Beta Sharer',          'Help 5 climbers by sharing beta on routes you''ve completed.',                                    'permanent', 'Log each time you coach someone through a move or sequence.',                  5,  14, 'beginner',      2)
  ) AS q(domain_name, badge_name, name, description, quest_type, criteria, target, days, level, sort)
  JOIN quest_domains d ON d.name = q.domain_name AND d.location_id = loc_id
  LEFT JOIN badges b ON b.name = q.badge_name AND b.location_id = loc_id;

  RAISE NOTICE 'Seeded progressions for location %', loc_id;
END $$;
