ALTER TABLE "remote_data" DROP CONSTRAINT "remote_data_pkey";
ALTER TABLE "remote_data" ADD PRIMARY KEY ("origin", "start", "end");
