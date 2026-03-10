export function PlaceholderPage({ title }: { title: string }) {
  return (
    <section className="card">
      <h1>{title}</h1>
      <p>本页骨架已创建，后续按模块继续补全。</p>
    </section>
  )
}
