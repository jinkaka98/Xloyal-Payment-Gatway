import Link from "next/link";
import { ArrowLeft, FileQuestion } from "lucide-react";
export default function NotFound() { return <div className="page"><div className="error-state"><FileQuestion size={32} /><h1>Invoice not found</h1><p>This invoice may have been removed or the identifier is incorrect.</p><Link className="button" href="/invoices"><ArrowLeft size={16} />Return to invoices</Link></div></div>; }
