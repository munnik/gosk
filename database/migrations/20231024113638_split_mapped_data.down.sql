DROP VIEW "mapped_data";
ALTER TABLE "mapped_data_matching_origin" RENAME TO "mapped_data";
--  mapped_data_other_origin is not dropped because that would result in data loss and inserting it back into mapped data would be too slow