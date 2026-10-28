import Link from "next/link";

export function Brand() {
  return (
    <Link href="/" className="brand" aria-label="SG-1 home">
      <span className="brand-mark">
        S<span>1</span>
      </span>
      <span>
        SG-1<span className="brand-label">INTERNAL STUDIO</span>
      </span>
    </Link>
  );
}
