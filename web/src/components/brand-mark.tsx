import Image from "next/image";

export function BrandMark({ size = 28 }: { size?: number }) {
  return (
    <Image
      src="/brand-mark.svg"
      alt=""
      aria-hidden="true"
      width={size}
      height={size}
      className="brand-mark-image"
      priority
    />
  );
}
