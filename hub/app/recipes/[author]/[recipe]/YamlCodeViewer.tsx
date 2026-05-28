"use client";

import { useState } from "react";
import { Copy, Check, FileCode } from "lucide-react";
import { toast } from "sonner";

interface YamlCodeViewerProps {
  yaml: string;
  filename: string;
}

export default function YamlCodeViewer({ yaml, filename }: { yaml: string; filename: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(yaml);
      setCopied(true);
      toast.success("YAML copied to clipboard!");
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error("Failed to copy YAML: ", err);
    }
  };

  const lines = yaml.split("\n");

  return (
    <div className="mt-16 w-full flex flex-col font-mono text-xs text-zinc-300 bg-[#111110] border border-zinc-300 dark:border-zinc-800 rounded-none relative overflow-hidden shadow-2xl">
      {/* SVG Corner L-Brackets */}
      <svg viewBox="0 0 12 12" className="absolute -top-[1px] -left-[1px] w-2.5 h-2.5 fill-black dark:fill-white pointer-events-none z-10">
        <path d="M 0,12 L 0,0 L 12,0 L 12,1 L 4,1 Q 1,1 1,4 L 1,12 Z" />
      </svg>
      <svg viewBox="0 0 12 12" className="absolute -top-[1px] -right-[1px] w-2.5 h-2.5 fill-black dark:fill-white scale-x-[-1] pointer-events-none z-10">
        <path d="M 0,12 L 0,0 L 12,0 L 12,1 L 4,1 Q 1,1 1,4 L 1,12 Z" />
      </svg>
      <svg viewBox="0 0 12 12" className="absolute -bottom-[1px] -left-[1px] w-2.5 h-2.5 fill-black dark:fill-white scale-y-[-1] pointer-events-none z-10">
        <path d="M 0,12 L 0,0 L 12,0 L 12,1 L 4,1 Q 1,1 1,4 L 1,12 Z" />
      </svg>
      <svg viewBox="0 0 12 12" className="absolute -bottom-[1px] -right-[1px] w-2.5 h-2.5 fill-black dark:fill-white scale-x-[-1] scale-y-[-1] pointer-events-none z-10">
        <path d="M 0,12 L 0,0 L 12,0 L 12,1 L 4,1 Q 1,1 1,4 L 1,12 Z" />
      </svg>

      {/* Header Bar */}
      <div className="flex items-center justify-between px-4 py-2 border-b border-zinc-250 dark:border-zinc-900 bg-zinc-200 dark:bg-zinc-950 select-none">
        <div className="flex items-center gap-2">
          <FileCode className="w-3.5 h-3.5 text-zinc-500" />
          <span className="text-[10px] text-zinc-700 dark:text-zinc-400 font-bold uppercase tracking-wider">{filename}</span>
        </div>
        <button
          onClick={handleCopy}
          className="flex items-center gap-1.5 px-2.5 py-1 bg-zinc-100 hover:bg-zinc-250 dark:bg-zinc-900 dark:hover:bg-zinc-800 border border-zinc-300 dark:border-zinc-800 hover:border-zinc-400 dark:hover:border-zinc-700 text-zinc-700 hover:text-black dark:text-zinc-450 dark:hover:text-zinc-200 transition-all duration-150 rounded-md text-[9px] font-mono uppercase font-bold tracking-wider cursor-pointer"
        >
          {copied ? (
            <>
              <Check className="w-3 h-3 text-emerald-500" />
              <span>Copied</span>
            </>
          ) : (
            <>
              <Copy className="w-3 h-3" />
              <span>Copy YAML</span>
            </>
          )}
        </button>
      </div>

      {/* Code Area */}
      <div className="flex overflow-x-auto max-h-[480px] bg-[#111110] scrollbar-thin">
        {/* Line Numbers */}
        <div className="py-4 px-3 bg-zinc-950/20 text-zinc-600 text-right select-none border-r border-zinc-200 dark:border-zinc-900 min-w-[36px] font-mono text-[10px] leading-5">
          {lines.map((_, i) => (
            <div key={i}>{i + 1}</div>
          ))}
        </div>

        {/* Highlighted Code */}
        <pre className="py-4 px-4 text-[11px] leading-5 font-mono text-zinc-300 overflow-x-auto w-full select-text whitespace-pre bg-transparent">
          <code>
            {lines.map((line, i) => {
              if (line.trim().startsWith("#")) {
                return (
                  <span key={i} className="text-zinc-550 dark:text-zinc-600 block">
                    {line}
                  </span>
                );
              }
              const colonIdx = line.indexOf(":");
              if (colonIdx !== -1) {
                const key = line.substring(0, colonIdx);
                const value = line.substring(colonIdx);
                return (
                  <span key={i} className="block">
                    <span className="text-blue-600 dark:text-blue-400 font-medium">{key}</span>
                    <span className="text-zinc-500">{value.substring(0, 1)}</span>
                    <span className="text-amber-600 dark:text-amber-500 font-semibold">{value.substring(1)}</span>
                  </span>
                );
              }
              return (
                <span key={i} className="block">
                  {line}
                </span>
              );
            })}
          </code>
        </pre>
      </div>
    </div>
  );
}
