package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

func TestReconcile(t *testing.T) {
	t.Parallel()

	single := func(local, remote, base string) entities.SyncEntry {
		snapshot := func(checksum string) map[string]string {
			if checksum == "" {
				return map[string]string{}
			}
			return map[string]string{"rules/go.md": checksum}
		}
		entries := entities.Reconcile(snapshot(local), snapshot(remote), snapshot(base))
		if len(entries) != 1 {
			return entities.SyncEntry{}
		}
		return entries[0]
	}

	t.Run("should report no action when both sides hold the same content", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "sha256:a", "sha256:a", "sha256:old"

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionNone, entry.Action)
		assert.Empty(t, entry.Reason)
	})

	t.Run("should forget the base entry when the file is gone on both sides", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "", "", "sha256:a"

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionForget, entry.Action)
	})

	t.Run("should push when the file exists only locally and was never synced", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "sha256:a", "", ""

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionPush, entry.Action)
	})

	t.Run("should pull when the file exists only in the repo and was never synced", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "", "sha256:b", ""

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionPull, entry.Action)
	})

	t.Run("should report a both-added conflict when never synced and the copies differ", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "sha256:a", "sha256:b", ""

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionConflict, entry.Action)
		assert.Equal(t, entities.ConflictBothAdded, entry.Reason)
	})

	t.Run("should delete locally when the repo deleted the file and the local copy is unchanged", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "sha256:a", "", "sha256:a"

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionDeleteLocal, entry.Action)
	})

	t.Run("should report a conflict when the repo deleted the file but it changed locally", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "sha256:new", "", "sha256:a"

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionConflict, entry.Action)
		assert.Equal(t, entities.ConflictLocalModifiedRemoteDeleted, entry.Reason)
	})

	t.Run(
		"should delete in the repo when the file was deleted locally and the repo copy is unchanged",
		func(t *testing.T) {
			t.Parallel()
			// given
			local, remote, base := "", "sha256:a", "sha256:a"

			// when
			entry := single(local, remote, base)

			// then
			assert.Equal(t, entities.SyncActionDeleteRepo, entry.Action)
		},
	)

	t.Run("should report a conflict when the file was deleted locally but changed in the repo", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "", "sha256:new", "sha256:a"

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionConflict, entry.Action)
		assert.Equal(t, entities.ConflictLocalDeletedRemoteModified, entry.Reason)
	})

	t.Run("should push when only the local copy changed since the last sync", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "sha256:new", "sha256:a", "sha256:a"

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionPush, entry.Action)
	})

	t.Run("should pull when only the repo copy changed since the last sync", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "sha256:a", "sha256:new", "sha256:a"

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionPull, entry.Action)
	})

	t.Run("should report a both-modified conflict when both copies changed differently", func(t *testing.T) {
		t.Parallel()
		// given
		local, remote, base := "sha256:mine", "sha256:theirs", "sha256:a"

		// when
		entry := single(local, remote, base)

		// then
		assert.Equal(t, entities.SyncActionConflict, entry.Action)
		assert.Equal(t, entities.ConflictBothModified, entry.Reason)
		assert.Equal(t, "sha256:mine", entry.LocalChecksum)
		assert.Equal(t, "sha256:theirs", entry.RemoteChecksum)
		assert.Equal(t, "sha256:a", entry.BaseChecksum)
	})

	t.Run("should cover every path from all three snapshots sorted by path", func(t *testing.T) {
		t.Parallel()
		// given
		local := map[string]string{"skills/b/SKILL.md": "sha256:1"}
		remote := map[string]string{"agents/a.md": "sha256:2"}
		base := map[string]string{"rules/old.md": "sha256:3"}

		// when
		entries := entities.Reconcile(local, remote, base)

		// then
		require.Len(t, entries, 3)
		assert.Equal(t, "agents/a.md", entries[0].Path)
		assert.Equal(t, entities.SyncActionPull, entries[0].Action)
		assert.Equal(t, "rules/old.md", entries[1].Path)
		assert.Equal(t, entities.SyncActionForget, entries[1].Action)
		assert.Equal(t, "skills/b/SKILL.md", entries[2].Path)
		assert.Equal(t, entities.SyncActionPush, entries[2].Action)
	})

	t.Run("should return an empty plan when every snapshot is empty", func(t *testing.T) {
		t.Parallel()
		// given
		empty := map[string]string{}

		// when
		entries := entities.Reconcile(empty, empty, empty)

		// then
		assert.Empty(t, entries)
	})
}

func TestSyncEntry_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("should push when keeping a local copy that exists", func(t *testing.T) {
		t.Parallel()
		// given
		entry := entities.SyncEntry{
			Action:         entities.SyncActionConflict,
			LocalChecksum:  "sha256:a",
			RemoteChecksum: "sha256:b",
		}

		// when
		resolved := entry.Resolve(entities.ChoiceKeepLocal)

		// then
		assert.Equal(t, entities.SyncActionPush, resolved.Action)
	})

	t.Run("should delete in the repo when keeping a local deletion", func(t *testing.T) {
		t.Parallel()
		// given
		entry := entities.SyncEntry{Action: entities.SyncActionConflict, RemoteChecksum: "sha256:b"}

		// when
		resolved := entry.Resolve(entities.ChoiceKeepLocalAll)

		// then
		assert.Equal(t, entities.SyncActionDeleteRepo, resolved.Action)
	})

	t.Run("should pull when keeping a repo copy that exists", func(t *testing.T) {
		t.Parallel()
		// given
		entry := entities.SyncEntry{
			Action:         entities.SyncActionConflict,
			LocalChecksum:  "sha256:a",
			RemoteChecksum: "sha256:b",
		}

		// when
		resolved := entry.Resolve(entities.ChoiceKeepRemoteAll)

		// then
		assert.Equal(t, entities.SyncActionPull, resolved.Action)
	})

	t.Run("should delete locally when keeping a repo deletion", func(t *testing.T) {
		t.Parallel()
		// given
		entry := entities.SyncEntry{Action: entities.SyncActionConflict, LocalChecksum: "sha256:a"}

		// when
		resolved := entry.Resolve(entities.ChoiceKeepRemote)

		// then
		assert.Equal(t, entities.SyncActionDeleteLocal, resolved.Action)
	})

	t.Run("should leave the conflict untouched when skipping", func(t *testing.T) {
		t.Parallel()
		// given
		entry := entities.SyncEntry{
			Action:         entities.SyncActionConflict,
			LocalChecksum:  "sha256:a",
			RemoteChecksum: "sha256:b",
		}

		// when
		resolved := entry.Resolve(entities.ChoiceSkip)

		// then
		assert.Equal(t, entities.SyncActionConflict, resolved.Action)
	})
}

func TestSyncDirection_Allows(t *testing.T) {
	t.Parallel()

	t.Run("should allow only repo writes when outgoing", func(t *testing.T) {
		t.Parallel()
		// given
		direction := entities.SyncOutgoing

		// when / then
		assert.True(t, direction.Allows(entities.SyncActionPush))
		assert.True(t, direction.Allows(entities.SyncActionDeleteRepo))
		assert.False(t, direction.Allows(entities.SyncActionPull))
		assert.False(t, direction.Allows(entities.SyncActionDeleteLocal))
		assert.True(t, direction.Allows(entities.SyncActionNone))
	})

	t.Run("should allow only local writes when incoming", func(t *testing.T) {
		t.Parallel()
		// given
		direction := entities.SyncIncoming

		// when / then
		assert.False(t, direction.Allows(entities.SyncActionPush))
		assert.False(t, direction.Allows(entities.SyncActionDeleteRepo))
		assert.True(t, direction.Allows(entities.SyncActionPull))
		assert.True(t, direction.Allows(entities.SyncActionDeleteLocal))
	})

	t.Run("should allow everything when syncing both ways", func(t *testing.T) {
		t.Parallel()
		// given
		direction := entities.SyncBoth

		// when / then
		assert.True(t, direction.Allows(entities.SyncActionPush))
		assert.True(t, direction.Allows(entities.SyncActionPull))
		assert.True(t, direction.Allows(entities.SyncActionDeleteRepo))
		assert.True(t, direction.Allows(entities.SyncActionDeleteLocal))
	})
}

func TestParseSyncPreference(t *testing.T) {
	t.Parallel()

	t.Run("should accept every documented preference", func(t *testing.T) {
		t.Parallel()
		// given
		values := []string{"", "local", "remote", "newer"}

		for _, value := range values {
			// when
			preference, err := entities.ParseSyncPreference(value)

			// then
			require.NoError(t, err)
			assert.Equal(t, entities.SyncPreference(value), preference)
		}
	})

	t.Run("should return an error when the preference is unknown", func(t *testing.T) {
		t.Parallel()
		// given
		value := "mine"

		// when
		_, err := entities.ParseSyncPreference(value)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mine")
	})
}

func TestConflictDetail_NewerSide(t *testing.T) {
	t.Parallel()

	earlier := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Hour)

	t.Run("should pick local when the local copy changed last", func(t *testing.T) {
		t.Parallel()
		// given
		detail := entities.ConflictDetail{LocalModified: later, RemoteModified: earlier}

		// when
		side := detail.NewerSide()

		// then
		assert.Equal(t, entities.ChoiceKeepLocal, side)
	})

	t.Run("should pick remote when the repo copy changed last", func(t *testing.T) {
		t.Parallel()
		// given
		detail := entities.ConflictDetail{LocalModified: earlier, RemoteModified: later}

		// when
		side := detail.NewerSide()

		// then
		assert.Equal(t, entities.ChoiceKeepRemote, side)
	})

	t.Run("should skip when a timestamp is unknown", func(t *testing.T) {
		t.Parallel()
		// given
		detail := entities.ConflictDetail{LocalModified: later}

		// when
		side := detail.NewerSide()

		// then
		assert.Equal(t, entities.ChoiceSkip, side)
	})

	t.Run("should skip when both sides changed at the same time", func(t *testing.T) {
		t.Parallel()
		// given
		detail := entities.ConflictDetail{LocalModified: earlier, RemoteModified: earlier}

		// when
		side := detail.NewerSide()

		// then
		assert.Equal(t, entities.ChoiceSkip, side)
	})
}
