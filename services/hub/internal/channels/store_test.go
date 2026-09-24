package channels

import (
	"context"
	"testing"
)

func TestEnsureIsIdempotentAndKindScoped(t *testing.T) {
	s, _ := stores(t)
	ctx := context.Background()

	ref := uniqueName("forgejo")
	first, err := s.Ensure(ctx, KindConnector, ref, "Forgejo", "Events from Forgejo")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	second, err := s.Ensure(ctx, KindConnector, ref, "Forgejo Renamed", "changed")
	if err != nil {
		t.Fatalf("ensure twice: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("ensure created a second channel: %s vs %s", first.ID, second.ID)
	}
	if second.Name != "Forgejo Renamed" {
		t.Errorf("ensure must refresh the display name from the entity, got %q", second.Name)
	}

	// The same ref under a different kind is a different stream: a connector and
	// an agent may share a label.
	inbox, err := s.Ensure(ctx, KindAgent, ref, ref, DefaultDescription(KindAgent, ref))
	if err != nil {
		t.Fatalf("ensure agent channel: %v", err)
	}
	if inbox.ID == first.ID {
		t.Fatal("connector and agent channels for the same ref collided")
	}
	if byEntity, err := s.GetByEntity(ctx, KindAgent, ref); err != nil || byEntity.ID != inbox.ID {
		t.Fatalf("GetByEntity(agent, %s) = %v, %v", ref, byEntity, err)
	}
}

func TestEnsureClearsOrphanFlag(t *testing.T) {
	s, _ := stores(t)
	ctx := context.Background()
	ref := uniqueName("orphan")

	seed(t, s, KindConnector, ref)
	// The registry stops listing it: the channel is orphaned.
	if n, err := s.MarkOrphans(ctx, KindConnector, []string{}); err != nil {
		t.Fatalf("mark orphans: %v", err)
	} else if n == 0 {
		t.Fatal("expected at least one orphaned connector channel")
	}
	ch, err := s.GetByEntity(ctx, KindConnector, ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ch.Orphaned {
		t.Fatal("channel should be flagged orphaned")
	}

	// It comes back: Ensure must clear the flag, not leave a stale marker.
	again, err := s.Ensure(ctx, KindConnector, ref, ref, "x")
	if err != nil {
		t.Fatalf("re-ensure: %v", err)
	}
	if again.Orphaned {
		t.Error("re-provisioned channel still flagged orphaned")
	}
}

func TestMarkOrphansOnlyTouchesItsKind(t *testing.T) {
	s, _ := stores(t)
	ctx := context.Background()
	ref := uniqueName("kind-scoped")

	connector := seed(t, s, KindConnector, ref)
	inbox := seed(t, s, KindAgent, ref)

	if _, err := s.MarkOrphans(ctx, KindConnector, []string{}); err != nil {
		t.Fatalf("mark orphans: %v", err)
	}
	if ch, _ := s.Get(ctx, connector); !ch.Orphaned {
		t.Error("connector channel not orphaned")
	}
	if ch, _ := s.Get(ctx, inbox); ch.Orphaned {
		t.Error("agent channel orphaned by a connector pass")
	}
}

func TestUpdateAndDeleteGuards(t *testing.T) {
	s, _ := stores(t)
	ctx := context.Background()

	provisioned := seed(t, s, KindConnector, uniqueName("guarded"))
	newName := "should not apply"
	if _, err := s.Update(ctx, provisioned, &newName, nil); err != ErrNotCustom {
		t.Fatalf("renaming a provisioned channel: got %v, want ErrNotCustom", err)
	}
	if err := s.Delete(ctx, provisioned); err != ErrNotCustom {
		t.Fatalf("deleting a provisioned channel: got %v, want ErrNotCustom", err)
	}

	custom := seedCustom(t, s)
	name := "renamed group"
	desc := "renamed description"
	updated, err := s.Update(ctx, custom, &name, &desc)
	if err != nil {
		t.Fatalf("update custom: %v", err)
	}
	if updated.Name != name || updated.Description != desc {
		t.Fatalf("update returned %+v", updated)
	}
	// A nil field leaves the column alone — a partial update must not blank the
	// description.
	partial, err := s.Update(ctx, custom, nil, nil)
	if err != nil {
		t.Fatalf("partial update: %v", err)
	}
	if partial.Name != name || partial.Description != desc {
		t.Fatalf("partial update changed fields: %+v", partial)
	}

	if err := s.Delete(ctx, custom); err != nil {
		t.Fatalf("delete custom: %v", err)
	}
	if _, err := s.Get(ctx, custom); err != ErrNotFound {
		t.Fatalf("get deleted: got %v, want ErrNotFound", err)
	}
}

func TestDeleteRefusesWhileBridgesRemain(t *testing.T) {
	s, _ := stores(t)
	ctx := context.Background()

	group := seedCustom(t, s)
	connector := seed(t, s, KindConnector, uniqueName("in-use"))
	if _, err := s.CreateBridge(ctx, connector, group, ""); err != nil {
		t.Fatalf("create bridge: %v", err)
	}
	if err := s.Delete(ctx, group); err != ErrInUse {
		t.Fatalf("delete in use: got %v, want ErrInUse", err)
	}

	// Detaching the edge releases the channel. Use the far end as the path
	// channel: the edge may be detached from either side.
	bridges, err := s.ListBridges(ctx, group)
	if err != nil || len(bridges) != 1 {
		t.Fatalf("list bridges: %v, %d", err, len(bridges))
	}
	if err := s.DeleteBridge(ctx, bridges[0].ID); err != nil {
		t.Fatalf("delete bridge: %v", err)
	}
	if err := s.Delete(ctx, group); err != nil {
		t.Fatalf("delete after detach: %v", err)
	}
}

func TestCreateBridgeValidation(t *testing.T) {
	s, _ := stores(t)
	ctx := context.Background()

	group := seedCustom(t, s)
	connector := seed(t, s, KindConnector, uniqueName("bridge-src"))
	inbox := seed(t, s, KindAgent, uniqueName("bridge-dst"))
	other := seed(t, s, KindConnector, uniqueName("bridge-other"))

	if _, err := s.CreateBridge(ctx, group, group, ""); err != ErrSelfEdge {
		t.Errorf("self edge: got %v, want ErrSelfEdge", err)
	}
	// Two provisioned streams are connected by a trigger, never by a bridge —
	// otherwise routing would live in two places.
	if _, err := s.CreateBridge(ctx, connector, inbox, ""); err != ErrNoCustomEndpoint {
		t.Errorf("connector→agent: got %v, want ErrNoCustomEndpoint", err)
	}
	if _, err := s.CreateBridge(ctx, connector, other, ""); err != ErrNoCustomEndpoint {
		t.Errorf("connector→connector: got %v, want ErrNoCustomEndpoint", err)
	}
	// A missing endpoint is a 404, not a silent create.
	if _, err := s.CreateBridge(ctx, "ch-nope", group, ""); err != ErrNotFound {
		t.Errorf("missing endpoint: got %v, want ErrNotFound", err)
	}

	bridge, err := s.CreateBridge(ctx, connector, group, "")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	// With no label supplied the edge names itself after its endpoints, so the
	// graph is legible without another lookup.
	if bridge.Name == "" || bridge.FromChannel != connector || bridge.ToChannel != group {
		t.Fatalf("unexpected bridge: %+v", bridge)
	}
	if _, err := s.CreateBridge(ctx, connector, group, ""); err != ErrBridgeExists {
		t.Errorf("duplicate edge: got %v, want ErrBridgeExists", err)
	}

	// A cycle: group reaches inbox, so inbox→group would close the loop.
	if _, err := s.CreateBridge(ctx, group, inbox, ""); err != nil {
		t.Fatalf("attach group→inbox: %v", err)
	}
	if _, err := s.CreateBridge(ctx, inbox, group, ""); err != ErrCycle {
		t.Errorf("cycle: got %v, want ErrCycle", err)
	}
	// The same cycle reached through a longer path: inbox→otherGroup→connector→group.
	far := seedCustom(t, s)
	if _, err := s.CreateBridge(ctx, inbox, far, ""); err != nil {
		t.Fatalf("attach inbox→far: %v", err)
	}
	if _, err := s.CreateBridge(ctx, far, connector, ""); err != ErrCycle {
		t.Errorf("indirect cycle: got %v, want ErrCycle", err)
	}
}

func TestDeliveriesFollowBridgePathsAndSkipDelivered(t *testing.T) {
	s, _ := stores(t)
	ctx := context.Background()

	src := seed(t, s, KindConnector, uniqueName("fanout-src"))
	group := seedCustom(t, s)
	first := seed(t, s, KindAgent, uniqueName("fanout-a"))
	second := seed(t, s, KindAgent, uniqueName("fanout-b"))
	nested := seedCustom(t, s)

	mustAttach := func(from, to string) {
		t.Helper()
		if _, err := s.CreateBridge(ctx, from, to, "edge "+from+"→"+to); err != nil {
			t.Fatalf("attach %s→%s: %v", from, to, err)
		}
	}
	// src → group → {first, second}, group → nested → (nothing)
	mustAttach(src, group)
	mustAttach(group, first)
	mustAttach(group, second)
	mustAttach(group, nested)

	got, err := s.Deliveries(ctx, src)
	if err != nil {
		t.Fatalf("deliveries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 inbox deliveries over a two-hop path, got %d: %+v", len(got), got)
	}
	byChannel := map[string]Delivery{}
	for _, d := range got {
		byChannel[d.AgentChannel] = d
	}
	for _, inbox := range []string{first, second} {
		d, ok := byChannel[inbox]
		if !ok {
			t.Fatalf("expected a delivery into %s, got %+v", inbox, got)
		}
		if d.AgentName == "" {
			t.Errorf("delivery must carry the agent name to enqueue for: %+v", d)
		}
		if d.BridgeID == "" || d.BridgeName == "" {
			t.Errorf("delivery must name the subscription that moved it: %+v", d)
		}
	}
	// A nested grouping channel with no inbox at the end is not a delivery.
	if _, ok := byChannel[nested]; ok {
		t.Error("a custom channel is not a delivery target")
	}

	// A path with no inboxes at its end yields nothing rather than an error.
	empty, err := s.Deliveries(ctx, first)
	if err != nil || len(empty) != 0 {
		t.Fatalf("deliveries from a leaf: %v, %+v", err, empty)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestReachesAndReachingInto(t *testing.T) {
	s, _ := stores(t)
	ctx := context.Background()

	src := seed(t, s, KindConnector, uniqueName("reach-src"))
	group := seedCustom(t, s)
	nested := seedCustom(t, s)
	inbox := seed(t, s, KindAgent, uniqueName("reach-dst"))
	island := seed(t, s, KindConnector, uniqueName("reach-island"))

	for _, edge := range [][2]string{{src, group}, {group, nested}, {nested, inbox}} {
		if _, err := s.CreateBridge(ctx, edge[0], edge[1], ""); err != nil {
			t.Fatalf("attach: %v", err)
		}
	}

	reaches, err := s.Reaches(ctx, src, inbox)
	if err != nil || !reaches {
		t.Fatalf("src should reach inbox through two hops: %v %v", reaches, err)
	}
	if reaches, _ := s.Reaches(ctx, island, inbox); reaches {
		t.Error("an unattached channel reaches nothing")
	}
	// Reaching a channel from itself is not a path; that question is what stops
	// a bridge from closing a loop.
	if reaches, _ := s.Reaches(ctx, src, src); reaches {
		t.Error("Reaches(from, from) must be false — only a real cycle counts")
	}

	// ReachingInto answers "what flows through this channel", which includes the
	// channel's own births — so the start is part of the result by design.
	into, err := s.ReachingInto(ctx, inbox)
	if err != nil {
		t.Fatalf("reaching into: %v", err)
	}
	want := map[string]bool{inbox: true, src: true, group: true, nested: true}
	if len(into) != len(want) {
		t.Fatalf("reaching into = %v, want %v", into, keys(want))
	}
	for _, id := range into {
		if !want[id] {
			t.Errorf("unexpected channel in the reverse path: %s", id)
		}
	}
	if into, err = s.ReachingInto(ctx, island); err != nil || len(into) != 1 || into[0] != island {
		t.Fatalf("an unattached channel reaches only itself: %v %+v", err, into)
	}
}
