package bernard

import (
	"fmt"

	"github.com/l3uddz/bernard"
	"github.com/l3uddz/bernard/datastore"
	"github.com/l3uddz/bernard/datastore/sqlite"
)

// NewPostProcessBernardDiff returns a bernard Hook that reclassifies trashed
// and un-trashed items in diff before they are persisted to the datastore.
func NewPostProcessBernardDiff(driveID string, store *bds, diff *sqlite.Difference) bernard.Hook {
	hook := func(drive datastore.Drive, files []datastore.File, folders []datastore.Folder, removed []string) error {
		// dont include removes for files already known as trashed
		for i := 0; i < len(diff.RemovedFiles); i++ {
			df := diff.RemovedFiles[i]

			ef, err := store.GetFile(driveID, df.ID)
			if err != nil {
				return fmt.Errorf("retrieving file (id: %v): %w", df.ID, err)
			}

			switch {
			case ef.Trashed && df.Trashed:
				// this removed file was already known as trashed (removed to us)
				diff.RemovedFiles = append(diff.RemovedFiles[:i], diff.RemovedFiles[i+1:]...)
				i--
			default:
				// file was not previously trashed, keep in removed list
			}
		}

		// dont include removes for folders already known as trashed
		for i := 0; i < len(diff.RemovedFolders); i++ {
			df := diff.RemovedFolders[i]

			ef, err := store.GetFolder(driveID, df.ID)
			if err != nil {
				return fmt.Errorf("retrieving folder (id: %v): %w", df.ID, err)
			}

			switch {
			case ef.Trashed && df.Trashed:
				// this removed folder was already known as trashed (removed to us)
				diff.RemovedFolders = append(diff.RemovedFolders[:i], diff.RemovedFolders[i+1:]...)
				i--
			default:
				// folder was not previously trashed, keep in removed list
			}
		}

		// remove changed files/folders that were trashed or un-trashed
		reclassifyTrashed(
			&diff.ChangedFiles, &diff.AddedFiles, &diff.RemovedFiles,
			func(d sqlite.FileDifference) (datastore.File, datastore.File) { return d.Old, d.New },
			func(f datastore.File) bool { return f.Trashed },
		)
		reclassifyTrashed(
			&diff.ChangedFolders, &diff.AddedFolders, &diff.RemovedFolders,
			func(d sqlite.FolderDifference) (datastore.Folder, datastore.Folder) { return d.Old, d.New },
			func(f datastore.Folder) bool { return f.Trashed },
		)

		return nil
	}

	return hook
}

// reclassifyTrashed moves entries from changed into added or removed when their
// trash state has flipped between the old and new snapshot.
// getChange returns the (old, new) pair for each difference element D.
// isTrash reports whether a given item T is trashed.
func reclassifyTrashed[D, T any](
	changed *[]D,
	added, removed *[]T,
	getChange func(D) (T, T),
	isTrash func(T) bool,
) {
	for i := 0; i < len(*changed); i++ {
		ef, df := getChange((*changed)[i])
		switch {
		case isTrash(ef) && !isTrash(df):
			// existing state was trashed, but new state is not
			*added = append(*added, df)
			*changed = append((*changed)[:i], (*changed)[i+1:]...)
			i--
		case !isTrash(ef) && isTrash(df):
			// new state is trashed, existing state is not
			*removed = append(*removed, df)
			*changed = append((*changed)[:i], (*changed)[i+1:]...)
			i--
		default:
			// no trash state change, keep in changed list
		}
	}
}
