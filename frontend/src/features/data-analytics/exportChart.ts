import type * as XLSX from 'xlsx'

const xml = (value: string) => value.replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' })[char]!)

// SheetJS CE writes cells only; add the standard OOXML chart parts using its bundled ZIP utility.
export function addTrendChart(xlsx: typeof XLSX, buffer: ArrayBuffer, sheetNumber: number, points: { bucket: string; tokens: string }[], daily: boolean): ArrayBuffer {
  if (!points.length) return buffer
  const zip = xlsx.CFB.read(new Uint8Array(buffer), { type: 'buffer' })
  const read = (path: string) => new TextDecoder().decode(xlsx.CFB.find(zip, `/${path}`).content)
  const add = (path: string, content: string) => xlsx.CFB.utils.cfb_add(zip, `/${path}`, new TextEncoder().encode(content))
  const relationshipNS = 'http://schemas.openxmlformats.org/package/2006/relationships'
  const officeNS = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships'
  const drawingNS = 'http://schemas.openxmlformats.org/drawingml/2006'
  const sheetPath = `xl/worksheets/sheet${sheetNumber}.xml`
  add(sheetPath, read(sheetPath).replace('</worksheet>', `<drawing xmlns:r="${officeNS}" r:id="rIdTrendChart"/></worksheet>`))
  add(`xl/worksheets/_rels/sheet${sheetNumber}.xml.rels`, `<Relationships xmlns="${relationshipNS}"><Relationship Id="rIdTrendChart" Type="${officeNS}/drawing" Target="../drawings/drawing1.xml"/></Relationships>`)
  add('xl/drawings/drawing1.xml', `<xdr:wsDr xmlns:xdr="${drawingNS}/spreadsheetDrawing" xmlns:a="${drawingNS}/main"><xdr:twoCellAnchor><xdr:from><xdr:col>3</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>1</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:from><xdr:to><xdr:col>15</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>23</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:to><xdr:graphicFrame macro=""><xdr:nvGraphicFramePr><xdr:cNvPr id="2" name="Token趋势"/><xdr:cNvGraphicFramePr/></xdr:nvGraphicFramePr><xdr:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></xdr:xfrm><a:graphic><a:graphicData uri="${drawingNS}/chart"><c:chart xmlns:c="${drawingNS}/chart" xmlns:r="${officeNS}" r:id="rIdChart"/></a:graphicData></a:graphic></xdr:graphicFrame><xdr:clientData/></xdr:twoCellAnchor></xdr:wsDr>`)
  add('xl/drawings/_rels/drawing1.xml.rels', `<Relationships xmlns="${relationshipNS}"><Relationship Id="rIdChart" Type="${officeNS}/chart" Target="../charts/chart1.xml"/></Relationships>`)
  const categories = points.map((p, i) => `<c:pt idx="${i}"><c:v>${xml(p.bucket)}</c:v></c:pt>`).join('')
  const values = points.map((p, i) => `<c:pt idx="${i}"><c:v>${xml(p.tokens)}</c:v></c:pt>`).join('')
  const series = `<c:ser><c:idx val="0"/><c:order val="0"/><c:tx><c:v>Token总数</c:v></c:tx><c:spPr>${daily ? '<a:ln w="25400"><a:solidFill><a:srgbClr val="0D9488"/></a:solidFill></a:ln>' : '<a:solidFill><a:srgbClr val="0D9488"/></a:solidFill>'}</c:spPr><c:cat><c:strRef><c:f>'Token趋势'!$A$2:$A$${points.length + 1}</c:f><c:strCache><c:ptCount val="${points.length}"/>${categories}</c:strCache></c:strRef></c:cat><c:val><c:numRef><c:f>'Token趋势'!$B$2:$B$${points.length + 1}</c:f><c:numCache><c:formatCode>#,##0</c:formatCode><c:ptCount val="${points.length}"/>${values}</c:numCache></c:numRef></c:val></c:ser>`
  const chartType = daily
    ? `<c:lineChart><c:grouping val="standard"/>${series}<c:marker val="1"/><c:axId val="1"/><c:axId val="2"/></c:lineChart>`
    : `<c:barChart><c:barDir val="col"/><c:grouping val="clustered"/>${series}<c:gapWidth val="80"/><c:axId val="1"/><c:axId val="2"/></c:barChart>`
  add('xl/charts/chart1.xml', `<c:chartSpace xmlns:c="${drawingNS}/chart" xmlns:a="${drawingNS}/main"><c:lang val="zh-CN"/><c:chart><c:title><c:tx><c:rich><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="zh-CN"/><a:t>Token趋势（${daily ? '按天' : '按月'}）</a:t></a:r></a:p></c:rich></c:tx></c:title><c:plotArea><c:layout/>${chartType}<c:catAx><c:axId val="1"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:axPos val="b"/><c:tickLblPos val="nextTo"/><c:crossAx val="2"/><c:crosses val="autoZero"/><c:auto val="1"/><c:lblAlgn val="ctr"/><c:lblOffset val="100"/></c:catAx><c:valAx><c:axId val="2"/><c:scaling><c:orientation val="minMax"/><c:min val="0"/></c:scaling><c:axPos val="l"/><c:majorGridlines/><c:numFmt formatCode="#,##0" sourceLinked="0"/><c:tickLblPos val="nextTo"/><c:crossAx val="1"/><c:crosses val="autoZero"/><c:crossBetween val="between"/></c:valAx></c:plotArea><c:plotVisOnly val="1"/></c:chart></c:chartSpace>`)
  add('[Content_Types].xml', read('[Content_Types].xml').replace('</Types>', '<Override PartName="/xl/drawings/drawing1.xml" ContentType="application/vnd.openxmlformats-officedocument.drawing+xml"/><Override PartName="/xl/charts/chart1.xml" ContentType="application/vnd.openxmlformats-officedocument.drawingml.chart+xml"/></Types>'))
  return new Uint8Array(xlsx.CFB.write(zip, { fileType: 'zip', compression: true })).buffer
}
