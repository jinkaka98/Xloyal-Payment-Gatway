"use client";
import { AlertTriangle, RotateCw } from "lucide-react";
export default function ErrorPage({ reset }: { error: Error & { digest?: string }; reset: () => void }) { return <div className="page"><div className="error-state"><AlertTriangle size={30} /><h1>Data could not be loaded</h1><p>The admin API did not return a usable response.</p><button className="button" onClick={reset}><RotateCw size={17} />Try again</button></div></div>; }
