import Link from 'next/link';

export default function HomePage() {
  return (
    <div className="flex flex-col justify-center text-center flex-1">
      <h1 className="text-2xl font-bold mb-4">Acme Docs</h1>
      <p>
        You can open{' '}
        <Link href="/docs" className="font-medium underline">
          /docs
        </Link>{' '}
        to read the documentation. Every page ends with Yuva&apos;s questions box and
        &ldquo;Was this page helpful?&rdquo;.
      </p>
    </div>
  );
}
