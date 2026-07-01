// Table tests for role resolution — realRoleAt / effectiveRoleAt /
// roleRankAt. These mirror the Go semantics in
// internal/middleware/websession.go::bestRole plus the SPA-side rules:
//   - an org-wide membership (location_id null) counts at every location
//   - a location-scoped membership counts only at its own location
//   - is_app_admin promotes to org_admin
//   - view_as only ever downgrades, never elevates

import { beforeEach, describe, expect, it } from 'vitest';
import type { MeResponse, MembershipShape } from '$lib/api/client';
import {
  authState,
  effectiveRoleAt,
  realRoleAt,
  roleRankAt,
} from '$lib/stores/auth.svelte';

const L1 = 'loc-1';
const L2 = 'loc-2';

let seq = 0;
function membership(role: string, locationId: string | null): MembershipShape {
  return {
    id: `m-${++seq}`,
    user_id: 'u1',
    org_id: 'o1',
    location_id: locationId,
    role,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  };
}

function me(
  memberships: MembershipShape[],
  opts: { isAppAdmin?: boolean; viewAs?: string } = {},
): MeResponse {
  return {
    user: {
      id: 'u1',
      email: 'x@example.com',
      display_name: 'X',
      is_app_admin: opts.isAppAdmin ?? false,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    memberships,
    view_as_role: opts.viewAs ?? '',
  };
}

beforeEach(() => {
  authState().me = null;
});

describe('realRoleAt', () => {
  const cases: {
    name: string;
    me: MeResponse | null;
    loc: string | null;
    want: string | null;
  }[] = [
    {
      name: 'no user resolves to null',
      me: null,
      loc: L1,
      want: null,
    },
    {
      name: 'org-wide membership (location_id null) counts at every location',
      me: me([membership('setter', null)]),
      loc: L1,
      want: 'setter',
    },
    {
      name: 'location-scoped membership counts at its own location',
      me: me([membership('head_setter', L1)]),
      loc: L1,
      want: 'head_setter',
    },
    {
      name: 'location-scoped membership does not leak to other locations',
      me: me([membership('head_setter', L1)]),
      loc: L2,
      want: null,
    },
    {
      name: 'best role wins when org-wide and location-scoped overlap',
      me: me([membership('setter', null), membership('gym_manager', L1)]),
      loc: L1,
      want: 'gym_manager',
    },
    {
      name: 'org-wide role still applies where the scoped one does not',
      me: me([membership('setter', null), membership('gym_manager', L1)]),
      loc: L2,
      want: 'setter',
    },
    {
      name: 'is_app_admin promotes a lower role to org_admin',
      me: me([membership('climber', L1)], { isAppAdmin: true }),
      loc: L1,
      want: 'org_admin',
    },
    {
      name: 'is_app_admin counts even with no memberships at all',
      me: me([], { isAppAdmin: true }),
      loc: L1,
      want: 'org_admin',
    },
    {
      name: 'a real org_admin membership needs no promotion',
      me: me([membership('org_admin', null)], { isAppAdmin: true }),
      loc: L1,
      want: 'org_admin',
    },
    {
      name: 'view_as does NOT affect the real role',
      me: me([membership('gym_manager', L1)], { viewAs: 'climber' }),
      loc: L1,
      want: 'gym_manager',
    },
  ];

  for (const c of cases) {
    it(c.name, () => {
      authState().me = c.me;
      expect(realRoleAt(c.loc)).toBe(c.want);
    });
  }
});

describe('effectiveRoleAt (view-as aware)', () => {
  const cases: {
    name: string;
    me: MeResponse | null;
    loc: string | null;
    want: string | null;
  }[] = [
    {
      name: 'no view_as passes the real role through',
      me: me([membership('head_setter', L1)]),
      loc: L1,
      want: 'head_setter',
    },
    {
      name: 'view_as downgrades below the real role',
      me: me([membership('gym_manager', L1)], { viewAs: 'climber' }),
      loc: L1,
      want: 'climber',
    },
    {
      name: 'view_as can never elevate above the real role',
      me: me([membership('setter', L1)], { viewAs: 'org_admin' }),
      loc: L1,
      want: 'setter',
    },
    {
      name: 'view_as equal to the real role is a no-op',
      me: me([membership('setter', L1)], { viewAs: 'setter' }),
      loc: L1,
      want: 'setter',
    },
    {
      name: 'an unknown view_as role is ignored',
      me: me([membership('gym_manager', L1)], { viewAs: 'superuser' }),
      loc: L1,
      want: 'gym_manager',
    },
    {
      name: 'view_as downgrades the app-admin promotion too',
      me: me([], { isAppAdmin: true, viewAs: 'setter' }),
      loc: L1,
      want: 'setter',
    },
    {
      name: 'no membership at the location resolves to null even with view_as',
      me: me([membership('gym_manager', L1)], { viewAs: 'climber' }),
      loc: L2,
      want: null,
    },
  ];

  for (const c of cases) {
    it(c.name, () => {
      authState().me = c.me;
      expect(effectiveRoleAt(c.loc)).toBe(c.want);
    });
  }
});

describe('roleRankAt', () => {
  const cases: {
    name: string;
    me: MeResponse | null;
    loc: string | null;
    want: number;
  }[] = [
    { name: 'no user ranks 0', me: null, loc: L1, want: 0 },
    {
      name: 'climber ranks 1',
      me: me([membership('climber', L1)]),
      loc: L1,
      want: 1,
    },
    {
      name: 'org-wide setter ranks 2 everywhere',
      me: me([membership('setter', null)]),
      loc: L2,
      want: 2,
    },
    {
      name: 'gym_manager ranks 4 at its location',
      me: me([membership('gym_manager', L1)]),
      loc: L1,
      want: 4,
    },
    {
      name: 'no membership at the location ranks 0',
      me: me([membership('gym_manager', L1)]),
      loc: L2,
      want: 0,
    },
    {
      name: 'app admin ranks 5',
      me: me([membership('climber', L1)], { isAppAdmin: true }),
      loc: L1,
      want: 5,
    },
    {
      name: 'view_as override rank applies',
      me: me([membership('gym_manager', L1)], { viewAs: 'setter' }),
      loc: L1,
      want: 2,
    },
  ];

  for (const c of cases) {
    it(c.name, () => {
      authState().me = c.me;
      expect(roleRankAt(c.loc)).toBe(c.want);
    });
  }
});
