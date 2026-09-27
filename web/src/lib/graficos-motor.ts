// ECharts só com o que o painel usa, carregado sob demanda (fica fora do pacote principal).
// Renderizador em canvas: não cria atributos style inline (a CSP não deixa).
import { BarChart, GaugeChart, HeatmapChart, LineChart, SankeyChart, TreemapChart } from "echarts/charts";
import {
  AriaComponent,
  CalendarComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
  VisualMapComponent,
} from "echarts/components";
import * as echarts from "echarts/core";
import ptBR from "echarts/lib/i18n/langPT-br.js";
import { CanvasRenderer } from "echarts/renderers";

echarts.use([
  BarChart,
  GaugeChart,
  HeatmapChart,
  LineChart,
  SankeyChart,
  TreemapChart,
  AriaComponent,
  CalendarComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
  VisualMapComponent,
  CanvasRenderer,
]);

echarts.registerLocale("PT-br", ptBR as Parameters<typeof echarts.registerLocale>[1]);

export { echarts };
