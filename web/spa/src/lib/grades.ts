// Default grade lists and circuit palette — used when the gym hasn't
// customized v_scale_range / yds_range / circuits, or when settings
// aren't available. Single source of truth for the SPA; keep in sync
// with internal/handler/web/route_form.go.

export const DEFAULT_V_GRADES = [
  'VB', 'V0', 'V1', 'V2', 'V3', 'V4', 'V5', 'V6', 'V7',
  'V8', 'V9', 'V10', 'V11', 'V12',
];

export const DEFAULT_YDS_GRADES = [
  '5.5', '5.6', '5.7', '5.8-', '5.8', '5.8+',
  '5.9-', '5.9', '5.9+',
  '5.10-', '5.10', '5.10+',
  '5.11-', '5.11', '5.11+',
  '5.12-', '5.12', '5.12+',
  '5.13-', '5.13', '5.13+',
  '5.14-', '5.14',
];

export const DEFAULT_CIRCUIT_COLORS = [
  { name: 'Red', hex: '#ef4444' },
  { name: 'Orange', hex: '#f59e0b' },
  { name: 'Yellow', hex: '#eab308' },
  { name: 'Green', hex: '#22c55e' },
  { name: 'Blue', hex: '#3b82f6' },
  { name: 'Purple', hex: '#a855f7' },
  { name: 'Pink', hex: '#ec4899' },
  { name: 'White', hex: '#ffffff' },
  { name: 'Black', hex: '#000000' },
];
