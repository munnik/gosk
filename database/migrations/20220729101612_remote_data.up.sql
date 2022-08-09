CREATE TABLE "remote_data" (
    "origin" TEXT NOT NULL, 
    "start" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "end" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "local" INTEGER NOT NULL DEFAULT -1, 
    "remote" INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY ("origin", "start")
);
