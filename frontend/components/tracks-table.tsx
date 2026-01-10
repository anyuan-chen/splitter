"use client";

import { AlertCircle, CheckCircle2, Loader2, XCircle } from "lucide-react";
import Link from "next/link";
import { Progress } from "@/components/ui/progress";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table";
import type { TrackState } from "@/lib/api";

interface TracksTableProps {
	tracks: TrackState[];
}

const StatusWithProgress = ({
	status,
	progress,
	error,
}: {
	status: string;
	progress: number;
	error?: string;
}) => {
	if (status === "completed") {
		return (
			<span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-green-700 border border-green-900 text-white text-xs font-medium">
				<CheckCircle2 className="w-3 h-3" /> Completed
			</span>
		);
	}

	if (status === "failed") {
		return (
			<div className="flex flex-col gap-1">
				<span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-red-700 border border-red-900 text-white text-xs font-medium w-fit">
					<XCircle className="w-3 h-3" /> Failed
				</span>
				{error && <span className="text-xs text-red-400">{error}</span>}
			</div>
		);
	}

	if (
		status === "in_progress" ||
		status === "downloading" ||
		status === "processing"
	) {
		return (
			<div className="flex items-center gap-2">
				<span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-blue-700 border border-blue-900 text-white text-xs font-medium">
					<Loader2 className="w-3 h-3 animate-spin" /> {Math.round(progress)}%
				</span>
				<Progress value={progress} className="h-1.5 w-16" />
			</div>
		);
	}

	return (
		<span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-gray-600 border border-gray-800 text-white text-xs font-medium">
			<AlertCircle className="w-3 h-3" /> Pending
		</span>
	);
};

export function TracksTable({ tracks }: TracksTableProps) {
	return (
		<div className="rounded-md">
			<Table className="table-fixed font-mono text-sm">
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
								<TableCell className="truncate" title={track.name}>
									<Link
										href={`/tracks/${track.track_id}`}
										className="hover:underline hover:text-primary"
									>
										{track.name}
									</Link>
								</TableCell>
								<TableCell className="truncate" title={track.artists}>
									{track.artists}
								</TableCell>
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
