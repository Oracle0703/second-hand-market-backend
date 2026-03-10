package common

type Actor struct {
	UserID     uint64
	UserType   string
	Role       string
	MerchantID uint64
	Scope      string
	SessionID  uint64
}

const actorKey = "actor"

func SetActor(ctx interface{ Set(string, any) }, actor Actor) {
	ctx.Set(actorKey, actor)
}

func GetActor(ctx interface{ Get(string) (any, bool) }) (Actor, bool) {
	v, ok := ctx.Get(actorKey)
	if !ok {
		return Actor{}, false
	}
	actor, ok := v.(Actor)
	return actor, ok
}
