"use client";

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Progress } from "@/components/ui/progress";
import { TrackState } from "@/lib/api";
import { Loader2, CheckCircle2, XCircle, AlertCircle } from "lucide-react";
import Link from "next/link";

interface TracksTableProps {
  tracks: TrackState[];
}

const StatusWithProgress = ({ 
  status, 
  progress, 
  error 
}: { 
  status: string, 
  progress: number, 
  error?: string 
}) => {
  if (status === "completed") {
    return <div className="flex items-center text-green-600 gap-2"><CheckCircle2 className="w-4 h-4" /> Completed</div>;
  }
  
  if (status === "failed") {
    return (
      <div className="flex flex-col gap-1 text-red-600">
        <div className="flex items-center gap-2"><XCircle className="w-4 h-4" /> Failed</div>
        <span className="text-xs text-red-500">{error}</span>
      </div>
    );
  }

  if (status === "in_progress" || status === "downloading" || status === "processing") {
    return (
      <div className="w-full max-w-[140px] space-y-1">
         <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span className="flex items-center gap-1"><Loader2 className="w-3 h-3 animate-spin" /> {Math.round(progress)}%</span>
         </div>
         <Progress value={progress} className="h-2" />
      </div>
    );
  }

  return <div className="text-muted-foreground flex items-center gap-2"><AlertCircle className="w-4 h-4" /> Pending</div>;
};

export function TracksTable({ tracks }: TracksTableProps) {
  return (
    <div className="rounded-md border">
      <Table className="table-fixed">
        <TableHeader>
          <TableRow>
            <TableHead className="w-[30%]">Track Name</TableHead>
            <TableHead className="w-[20%]">Artist(s)</TableHead>
            <TableHead className="w-[25%]">Download Status</TableHead>
            <TableHead className="w-[25%]">Demucs Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {tracks.length === 0 ? (
             <TableRow>
              <TableCell colSpan={4} className="h-24 text-center">
                No tracks found.
              </TableCell>
            </TableRow>
          ) : (
            tracks.map((track) => (
              <TableRow key={track.track_id}>
                <TableCell className="font-medium truncate" title={track.name}>
                  <Link 
                    href={`/tracks/${track.track_id}`} 
                    className="hover:underline hover:text-primary"
                  >
                    {track.name}
                  </Link>
                </TableCell>
                <TableCell className="truncate" title={track.artists}>{track.artists}</TableCell>
                <TableCell>
                  <StatusWithProgress 
                    status={track.download_status} 
                    progress={track.download_progress} 
                    error={track.download_error}
                  />
                </TableCell>
                <TableCell>
                  <StatusWithProgress 
                    status={track.demucs_status} 
                    progress={track.demucs_progress} 
                    error={track.demucs_error}
                  />
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}
