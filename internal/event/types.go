package event

// Event type constants. Start with what progressions needs;
// add more as features grow.
const (
	// Progressions
	QuestStarted   = "quest.started"
	QuestCompleted = "quest.completed"
	QuestAbandoned = "quest.abandoned"
	ProgressLogged = "progress.logged"
	BadgeEarned    = "badge.earned"

	// Routes (existing features — wire these up when ready)
	RouteCreated  = "route.created"
	RouteArchived = "route.archived"
	RouteSent     = "route.sent"  // climber sends a route
	RouteRated    = "route.rated" // climber rates a route

	// Membership (future)
	MemberJoined = "member.joined"
	MemberLeft   = "member.left"

	// Social (future)
	FollowCreated  = "follow.created"
	CommentCreated = "comment.created"
)
