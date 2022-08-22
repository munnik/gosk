ALTER TABLE mapped_data DISABLE ROW LEVEL SECURITY;
DROP POLICY read_mapped_data on mapped_data;
ALTER TABLE organization_vessels DISABLE ROW LEVEL SECURITY;
DROP POLICY read_vessels on organization_vessels;
DROP TABLE organization_vessels;

