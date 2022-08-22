CREATE TABLE organization_vessels (origin TEXT NOT NULL, organization TEXT NOT NULL, PRIMARY KEY("origin","organization"));
ALTER TABLE mapped_data ENABLE ROW LEVEL SECURITY;
ALTER TABLE organization_vessels ENABLE ROW LEVEL SECURITY;

CREATE POLICY read_mapped_data ON mapped_data FOR SELECT USING (origin IN (SELECT origin FROM organization_vessels WHERE organization = current_user));
CREATE POLICY read_vessels on organization_vessels FOR SELECT USING(organization = current_user);