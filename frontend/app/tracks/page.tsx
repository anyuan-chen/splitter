"use client";

import { useTracks } from "@/hooks/use-tracks";
import { TracksTable } from "@/components/tracks-table";
import { Button } from "@/components/ui/button";
import Link from "next/link";
import { ArrowLeft, Loader2 } from "lucide-react";

export default function TracksPage() {
  const { tracks, loading, error } = useTracks();

  return (
    <div className="container mx-auto py-10 px-4 min-h-screen">
      <div className="flex flex-col gap-8">
        <div className="flex items-center justify-between">
           <div className="flex items-center gap-4">
             <Link href="/">
               <Button variant="outline" size="icon">
                 <ArrowLeft className="w-4 h-4" />
               </Button>
             </Link>
             <h1 className="text-3xl font-bold">Track Status</h1>
           </div>
           
           <div className="text-sm text-muted-foreground">
              {tracks.length} Tracks
           </div>
        </div>

        {loading ? (
          <div className="flex justify-center items-center h-64">
            <Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
          </div>
        ) : error ? (
          <div className="text-red-500 p-4 border border-red-200 rounded-md bg-red-50 dark:bg-red-900/10">
            Error: {error}
          </div>
        ) : (
          <TracksTable tracks={tracks} />
        )}
      </div>
    </div>
  );
}
