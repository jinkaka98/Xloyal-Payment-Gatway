import { ArrowRight, LockKeyhole } from "lucide-react";
import { login } from "./actions";

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ error?: string }> }) {
  const { error } = await searchParams;
  return <main className="login-page">
    <section className="login-panel" aria-labelledby="login-heading">
      <div className="brand-row login-brand"><div className="brand-mark" aria-hidden="true">X</div><div><strong>Xloyal</strong><span>Payment operations</span></div></div>
      <div className="login-copy"><p className="eyebrow">Admin access</p><h1 id="login-heading">Sign in to the console</h1><p>Monitor QRIS payments, providers, and tenant activity.</p></div>
      <form className="login-form" action={login}>
        <label>Email address<input type="email" name="email" autoComplete="email" placeholder="you@company.id" required /></label>
        <label>Password<input type="password" name="password" autoComplete="current-password" placeholder="Enter your password" required /></label>
        {error && <p role="alert">Email or password is incorrect.</p>}
        <button type="submit" className="button button-primary">Sign in <ArrowRight size={17} /></button>
      </form>
      <p className="login-security"><LockKeyhole size={15} />Protected administrative environment</p>
    </section>
    <aside className="login-context"><div><span className="context-index">01 / OPERATIONS</span><blockquote>One control surface for every QRIS transaction.</blockquote><p>Provider-aware monitoring with complete audit visibility.</p></div><dl><div><dt>Environment</dt><dd>Production</dd></div><div><dt>Region</dt><dd>Indonesia</dd></div></dl></aside>
  </main>;
}
