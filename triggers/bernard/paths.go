package bernard

import (
	"fmt"
	"path/filepath"

	"github.com/l3uddz/bernard"
	"github.com/l3uddz/bernard/datastore"
	"github.com/l3uddz/bernard/datastore/sqlite"
)

// Paths holds the new and old folder paths produced by a Bernard diff hook.
type Paths struct {
	NewFolders []string
	OldFolders []string
}

// NewPathsHook creates a Bernard hook that collects new and old folder paths from a diff.
func NewPathsHook(driveID string, store *bds, diff *sqlite.Difference) (bernard.Hook, *Paths) {
	var paths Paths

	hook := func(drive datastore.Drive, files []datastore.File, folders []datastore.Folder, removed []string) error {
		// get folders from diff (that we are interested in)
		parents, err := getDiffFolders(store, driveID, diff)
		if err != nil {
			return fmt.Errorf("getting parents: %w", err)
		}

		// get roots from folders
		rootNewFolders, _ := datastore.RootFolders(parents.New)
		rootOldFolders, _ := datastore.RootFolders(parents.Old)

		// get new/changed paths
		for _, folder := range rootNewFolders {
			p, err := getFolderPath(store, driveID, folder.ID, parents.FolderMaps.Current)
			if err != nil {
				return fmt.Errorf("building folder path: %v: %w", folder.ID, err)
			}

			paths.NewFolders = append(paths.NewFolders, p)
		}

		// get removed paths
		for _, folder := range rootOldFolders {
			p, err := getFolderPath(store, driveID, folder.ID, parents.FolderMaps.Old)
			if err != nil {
				return fmt.Errorf("building old folder path: %v: %w", folder.ID, err)
			}

			paths.OldFolders = append(paths.OldFolders, p)
		}

		return nil
	}

	return hook, &paths
}

type diffFolderMaps struct {
	Current map[string]datastore.Folder
	Old     map[string]datastore.Folder
}

func getDiffFolderMaps(diff *sqlite.Difference) *diffFolderMaps {
	currentFolders := make(map[string]datastore.Folder)
	oldFolders := make(map[string]datastore.Folder)

	for i, f := range diff.AddedFolders {
		currentFolders[f.ID] = diff.AddedFolders[i]
		oldFolders[f.ID] = diff.AddedFolders[i]
	}

	for i, f := range diff.ChangedFolders {
		currentFolders[f.New.ID] = diff.ChangedFolders[i].New
		oldFolders[f.Old.ID] = diff.ChangedFolders[i].Old
	}

	for i, f := range diff.RemovedFolders {
		oldFolders[f.ID] = diff.RemovedFolders[i]
	}

	return &diffFolderMaps{
		Current: currentFolders,
		Old:     oldFolders,
	}
}

// Parents holds the old and new parent folders involved in a Bernard diff.
type Parents struct {
	New        []datastore.Folder
	Old        []datastore.Folder
	FolderMaps *diffFolderMaps
}

func getDiffFolders(store *bds, driveID string, diff *sqlite.Difference) (*Parents, error) {
	folderMaps := getDiffFolderMaps(diff)

	newParents := make(map[string]datastore.Folder)
	oldParents := make(map[string]datastore.Folder)

	// changed folders
	for _, folder := range diff.ChangedFolders {
		newParents[folder.New.ID] = folder.New
		oldParents[folder.Old.ID] = folder.Old
	}

	// removed folders
	for _, folder := range diff.RemovedFolders {
		oldParents[folder.ID] = folder
	}

	// added files
	for _, file := range diff.AddedFiles {
		folder, err := getFolder(store, driveID, file.Parent, folderMaps.Current)
		if err != nil {
			return nil, fmt.Errorf("added file: %w", err)
		}

		newParents[folder.ID] = *folder
	}

	// changed files
	for _, file := range diff.ChangedFiles {
		// current
		currentFolder, err := getFolder(store, driveID, file.New.Parent, folderMaps.Current)
		if err != nil {
			return nil, fmt.Errorf("changed new file: %w", err)
		}

		newParents[currentFolder.ID] = *currentFolder

		// old
		oldFolder, err := getFolder(store, driveID, file.Old.Parent, folderMaps.Old)
		if err != nil {
			return nil, fmt.Errorf("changed old file: %w", err)
		}

		oldParents[oldFolder.ID] = *oldFolder
	}

	// removed files
	for _, file := range diff.RemovedFiles {
		oldFolder, err := getFolder(store, driveID, file.Parent, folderMaps.Old)
		if err != nil {
			return nil, fmt.Errorf("removed file: %w", err)
		}

		oldParents[oldFolder.ID] = *oldFolder
	}

	// create Parents object
	pathResult := &Parents{
		New:        make([]datastore.Folder, 0),
		Old:        make([]datastore.Folder, 0),
		FolderMaps: folderMaps,
	}

	for _, folder := range newParents {
		pathResult.New = append(pathResult.New, folder)
	}

	for _, folder := range oldParents {
		pathResult.Old = append(pathResult.Old, folder)
	}

	return pathResult, nil
}

func getFolder(store *bds, driveID, folderID string, folderMap map[string]datastore.Folder) (*datastore.Folder, error) {
	// find folder in map
	if folder, ok := folderMap[folderID]; ok {
		return &folder, nil
	}

	if folderID == driveID {
		folder := datastore.Folder{
			ID:      driveID,
			Name:    "",
			Parent:  "",
			Trashed: false,
		}

		folderMap[driveID] = folder
		return &folder, nil
	}

	// search datastore
	folder, err := store.GetFolder(driveID, folderID)
	if err != nil {
		return nil, fmt.Errorf("could not get folder: %v: %w", folderID, err)
	}

	// add folder to map
	folderMap[folder.ID] = *folder

	return folder, nil
}

func getFolderPath(store *bds, driveID, folderID string, folderMap map[string]datastore.Folder) (string, error) {
	path := ""

	// folderID == driveID
	if folderID == driveID {
		return "/", nil
	}

	// get top folder
	topFolder, ok := folderMap[folderID]
	if !ok {
		f, err := store.GetFolder(driveID, folderID)
		if err != nil {
			return "/" + path, fmt.Errorf("could not get folder %v: %w", folderID, err)
		}

		topFolder = *f
	}

	// set logic variables
	path = topFolder.Name
	nextFolderID := topFolder.Parent

	// get folder paths
	for nextFolderID != "" && nextFolderID != driveID {
		folderEntry, ok := folderMap[nextFolderID]
		if !ok {
			df, err := store.GetFolder(driveID, nextFolderID)
			if err != nil {
				return "/" + path, fmt.Errorf("could not get folder %v: %w", nextFolderID, err)
			}

			folderEntry = *df
			folderMap[folderEntry.ID] = folderEntry
		}

		path = filepath.Join(folderEntry.Name, path)
		nextFolderID = folderEntry.Parent
	}

	return "/" + path, nil
}
