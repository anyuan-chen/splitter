"use client";

import { Pause, Play, Volume2, VolumeX } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { cn } from "@/lib/utils";

interface AudioPlayerProps {
	src: string;
	className?: string;
}

export function AudioPlayer({ src, className }: AudioPlayerProps) {
	const [isPlaying, setIsPlaying] = useState(false);
	const [progress, setProgress] = useState(0);
	const [duration, setDuration] = useState(0);
	const [currentTime, setCurrentTime] = useState(0);
	const [isMuted, setIsMuted] = useState(false);
	const audioRef = useRef<HTMLAudioElement>(null);

	useEffect(() => {
		const audio = audioRef.current;
		if (!audio) return;

		const updateProgress = () => {
			setCurrentTime(audio.currentTime);
			setProgress((audio.currentTime / audio.duration) * 100);
		};

		const handleLoadedMetadata = () => {
			setDuration(audio.duration);
		};

		const handleEnded = () => {
			setIsPlaying(false);
			setProgress(0);
			setCurrentTime(0);
		};

		audio.addEventListener("timeupdate", updateProgress);
		audio.addEventListener("loadedmetadata", handleLoadedMetadata);
		audio.addEventListener("ended", handleEnded);

		return () => {
			audio.removeEventListener("timeupdate", updateProgress);
			audio.removeEventListener("loadedmetadata", handleLoadedMetadata);
			audio.removeEventListener("ended", handleEnded);
		};
	}, []);

	const togglePlay = () => {
		if (audioRef.current) {
			if (isPlaying) {
				audioRef.current.pause();
			} else {
				audioRef.current.play();
			}
			setIsPlaying(!isPlaying);
		}
	};

	const toggleMute = () => {
		if (audioRef.current) {
			audioRef.current.muted = !isMuted;
			setIsMuted(!isMuted);
		}
	};

	const handleSeek = (e: React.MouseEvent<HTMLDivElement>) => {
		if (audioRef.current && duration) {
			const rect = e.currentTarget.getBoundingClientRect();
			const x = e.clientX - rect.left;
			const pct = x / rect.width;
			const newTime = pct * duration;
			audioRef.current.currentTime = newTime;
			setCurrentTime(newTime);
			setProgress(pct * 100);
		}
	};

	const formatTime = (time: number) => {
		const mins = Math.floor(time / 60);
		const secs = Math.floor(time % 60);
		return `${mins}:${secs.toString().padStart(2, "0")}`;
	};

	return (
		<div
			className={cn(
				"flex items-center gap-3 bg-zinc-900/50 backdrop-blur-sm border border-zinc-800 rounded-lg p-2 w-full max-w-md",
				className,
			)}
		>
			<audio ref={audioRef} src={src} />

			<Button
				variant="ghost"
				size="icon"
				className="h-8 w-8 text-zinc-400 hover:text-white hover:bg-zinc-800"
				onClick={togglePlay}
			>
				{isPlaying ? (
					<Pause className="h-4 w-4" />
				) : (
					<Play className="h-4 w-4 ml-0.5" />
				)}
			</Button>

			<div className="flex-1 flex flex-col gap-1">
				<div
					className="relative h-6 w-full cursor-pointer group flex items-center"
					onClick={handleSeek}
				>
					<div className="h-1.5 w-full bg-zinc-800 rounded-full overflow-hidden">
						<div
							className="h-full bg-blue-500 rounded-full group-hover:bg-blue-400 transition-all"
							style={{ width: `${progress}%` }}
						/>
					</div>
				</div>
				<div className="flex justify-between text-[10px] text-zinc-500 font-mono">
					<span>{formatTime(currentTime)}</span>
					<span>{formatTime(duration)}</span>
				</div>
			</div>

			<Button
				variant="ghost"
				size="icon"
				className="h-8 w-8 text-zinc-400 hover:text-white hover:bg-zinc-800"
				onClick={toggleMute}
			>
				{isMuted ? (
					<VolumeX className="h-4 w-4" />
				) : (
					<Volume2 className="h-4 w-4" />
				)}
			</Button>
		</div>
	);
}
