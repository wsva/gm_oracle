package main

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	wl_db "github.com/wsva/lib_go_db"
	mlib "github.com/wsva/monitor_lib_go"
)

type MDInfo struct {
	MD        mlib.MDOracle
	Lock      sync.Mutex
	WG        sync.WaitGroup
	ErrorList []string
}

func (m *MDInfo) AddError(err string) {
	m.Lock.Lock()
	m.ErrorList = append(m.ErrorList, err)
	m.Lock.Unlock()
}

func checkConnectivity(db *wl_db.DB) (bool, error) {
	sqltext := `SELECT STATUS FROM V$INSTANCE`
	row, err := db.QueryRow(sqltext)
	if err != nil {
		return false, err
	}
	var f1 sql.NullString
	err = row.Scan(&f1)
	if err != nil {
		return false, err
	}
	if f1.String != "OPEN" {
		return false, errors.New("status is not open")
	}
	return true, nil
}

func checkArchiveLog(db *wl_db.DB, m *MDInfo) {
	defer m.WG.Done()
	sqltext := `select log_mode from v$database`
	row, err := db.QueryRow(sqltext)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckArchiveLogExist error: %v", err))
		return
	}
	var f1 sql.NullString
	err = row.Scan(&f1)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckArchiveLogExist error: %v", err))
		return
	}
	if f1.String == "NOARCHIVELOG" {
		m.Lock.Lock()
		m.MD.ArchiveLogExist = false
		m.Lock.Unlock()
		return
	}

	sqltext = `SELECT 
ROUND(SPACE_LIMIT / 1024 / 1024 / 1024, 0) Size,
ROUND(SPACE_USED * 100 / SPACE_LIMIT, 0) Used 
FROM V$RECOVERY_FILE_DEST`
	row, err = db.QueryRow(sqltext)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckArchiveLog error: %v", err))
		return
	}
	var g1, g2 sql.NullInt32
	err = row.Scan(&g1, &g2)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckArchiveLog error: %v", err))
		return
	}
	m.Lock.Lock()
	m.MD.ArchiveLog = mlib.ArchiveLog{
		Size: int(g1.Int32),
		Used: int(g2.Int32),
	}
}

func checkASM(db *wl_db.DB, m *MDInfo) {
	defer m.WG.Done()
	sqltext := `SELECT COUNT(1) FROM V$ASM_DISKGROUP`
	row, err := db.QueryRow(sqltext)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckASMExist error: %v", err))
		return
	}
	var f1 sql.NullInt32
	err = row.Scan(&f1)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckASMExist error: %v", err))
		return
	}
	if f1.Int32 == 0 {
		m.Lock.Lock()
		m.MD.ASMExist = false
		m.Lock.Unlock()
		return
	}

	sqltext = `SELECT NAME, 
ROUND(TOTAL_MB / 1024, 0) Size,
ROUND(1 - (FREE_MB / TOTAL_MB), 2) * 100 Used
FROM V$ASM_DISKGROUP`
	rows, err := db.Query(sqltext)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckASM error: %v", err))
		return
	}
	var result []mlib.ASM
	var g1 sql.NullString
	var g2, g3 sql.NullInt32
	for rows.Next() {
		err = rows.Scan(&g1, &g2, &g3)
		if err != nil {
			m.AddError(fmt.Sprintf("CheckASM error: %v", err))
			return
		}
		result = append(result, mlib.ASM{
			Name: g1.String,
			Size: int(g2.Int32),
			Used: int(g3.Int32),
		})
	}
	err = rows.Close()
	if err != nil {
		m.AddError(fmt.Sprintf("CheckASM error: %v", err))
		return
	}
	m.Lock.Lock()
	m.MD.ASMList = result
	m.Lock.Unlock()
}

func checkTableSpace(db *wl_db.DB, excludeList []string, m *MDInfo) {
	defer m.WG.Done()
	sqltext := `SELECT T.NAME,
(SELECT ROUND(P.VALUE * S.TABLESPACE_SIZE / 1024 / 1024 / 1024, 0)
	FROM V$PARAMETER P
	WHERE P.NAME = 'db_block_size') Size,
ROUND((S.TABLESPACE_USEDSIZE / S.TABLESPACE_SIZE) * 100, 0) Used
FROM SYS.WRH$_TABLESPACE_SPACE_USAGE S, SYS.TS$ T
WHERE S.SNAP_ID = (SELECT MAX(SNAP_ID) FROM SYS.WRH$_TABLESPACE_SPACE_USAGE)
AND S.TABLESPACE_ID = T.TS#`
	rows, err := db.Query(sqltext)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckTableSpace error: %v", err))
		return
	}
	var result []mlib.TableSpace
	var f1 sql.NullString
	var f2, f3 sql.NullInt32
	for rows.Next() {
		err = rows.Scan(&f1, &f2, &f3)
		if err != nil {
			m.AddError(fmt.Sprintf("CheckTableSpace error: %v", err))
			return
		}
		result = append(result, mlib.TableSpace{
			Name: f1.String,
			Size: int(f2.Int32),
			Used: int(f3.Int32),
		})
	}
	err = rows.Close()
	if err != nil {
		m.AddError(fmt.Sprintf("CheckTableSpace error: %v", err))
		return
	}
	m.Lock.Lock()
	m.MD.TableSpaceList = result
	m.Lock.Unlock()
}

func checkTableLock(db *wl_db.DB, m *MDInfo) {
	defer m.WG.Done()
	sqltext := `SELECT A.OBJECT_NAME, B.ORACLE_USERNAME, count(1)
FROM DBA_OBJECTS A, V$LOCKED_OBJECT B
WHERE A.OBJECT_ID = B.OBJECT_ID
GROUP BY A.OBJECT_NAME, B.ORACLE_USERNAME`
	rows, err := db.Query(sqltext)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckTableLock error: %v", err))
		return
	}
	var result []mlib.TableLock
	var f1, f2 sql.NullString
	var f3 sql.NullInt32
	for rows.Next() {
		err = rows.Scan(&f1, &f2, &f3)
		if err != nil {
			m.AddError(fmt.Sprintf("CheckTableLock error: %v", err))
			return
		}
		result = append(result, mlib.TableLock{
			Name:     f1.String,
			Username: f2.String,
			Count:    int(f3.Int32),
		})
	}
	err = rows.Close()
	if err != nil {
		m.AddError(fmt.Sprintf("CheckTableLock error: %v", err))
		return
	}
	m.Lock.Lock()
	m.MD.TableLockList = result
	m.Lock.Unlock()
}

func checkPassword(db *wl_db.DB, m *MDInfo) {
	defer m.WG.Done()
	sqltext := `SELECT A.USERNAME FROM DBA_USERS A 
WHERE A.ACCOUNT_STATUS NOT LIKE '%LOCKED%' 
AND A.EXPIRY_DATE < SYSDATE + 7`
	rows, err := db.Query(sqltext)
	if err != nil {
		m.AddError(fmt.Sprintf("CheckPassword error: %v", err))
		return
	}
	var result []string
	var f1 sql.NullString
	for rows.Next() {
		err = rows.Scan(&f1)
		if err != nil {
			m.AddError(fmt.Sprintf("CheckPassword error: %v", err))
			return
		}
		result = append(result, f1.String)
	}
	err = rows.Close()
	if err != nil {
		m.AddError(fmt.Sprintf("CheckPassword error: %v", err))
		return
	}
	m.Lock.Lock()
	m.MD.PasswordExpireList = result
	m.Lock.Unlock()
}
