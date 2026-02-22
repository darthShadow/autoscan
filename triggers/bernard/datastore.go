package bernard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/l3uddz/bernard/datastore"
	"github.com/l3uddz/bernard/datastore/sqlite"
)

type bds struct {
	*sqlite.Datastore
}

const sqlSelectFile = `SELECT id, name, parent, size, md5, trashed FROM file WHERE drive = $1 AND id = $2 LIMIT 1`

func (d *bds) GetFile(driveID, fileID string) (*datastore.File, error) {
	fileRec := new(datastore.File)

	row := d.DB.QueryRowContext(context.Background(), sqlSelectFile, driveID, fileID)
	err := row.Scan(&fileRec.ID, &fileRec.Name, &fileRec.Parent, &fileRec.Size, &fileRec.MD5, &fileRec.Trashed)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("%v: file not found: %w", fileID, sql.ErrNoRows)
	case err != nil:
		return nil, fmt.Errorf("scan file row: %w", err)
	default:
	}

	return fileRec, nil
}

const sqlSelectFolder = `SELECT id, name, trashed, parent FROM folder WHERE drive = $1 AND id = $2 LIMIT 1`

func (d *bds) GetFolder(driveID, folderID string) (*datastore.Folder, error) {
	folderRec := new(datastore.Folder)

	row := d.DB.QueryRowContext(context.Background(), sqlSelectFolder, driveID, folderID)
	err := row.Scan(&folderRec.ID, &folderRec.Name, &folderRec.Trashed, &folderRec.Parent)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("%v: folder not found: %w", folderID, sql.ErrNoRows)
	case err != nil:
		return nil, fmt.Errorf("scan folder row: %w", err)
	default:
	}

	return folderRec, nil
}

const sqlSelectDrive = `SELECT d.id, f.name, d.pageToken` +
	` FROM drive d JOIN folder f ON f.id = d.id WHERE d.id = $1 LIMIT 1`

func (d *bds) GetDrive(driveID string) (*datastore.Drive, error) {
	drv := new(datastore.Drive)

	row := d.DB.QueryRowContext(context.Background(), sqlSelectDrive, driveID)
	err := row.Scan(&drv.ID, &drv.Name, &drv.PageToken)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("%v: drive not found: %w", driveID, sql.ErrNoRows)
	case err != nil:
		return nil, fmt.Errorf("scan drive row: %w", err)
	default:
	}

	return drv, nil
}
