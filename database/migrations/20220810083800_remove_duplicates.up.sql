DELETE FROM "mapped_data"
WHERE ctid IN (
	SELECT ctid
	FROM (
		SELECT ctid, ROW_NUMBER() OVER(PARTITION BY "time", "collector", "context", "origin", "path", "value") AS row_num 
		FROM "mapped_data"
	) t
WHERE t.row_num > 1);
