drop view "mapped_data";
alter table "mapped_data_matching_origin" rename to "mapped_data";
--  mapped_data_other_origin is not dropped because that would result in dataloss and inserting it back into mapped data would be too slow