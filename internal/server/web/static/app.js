// Dashboard chart wiring: reads the JSON block the SSR template emits and
// renders it with ECharts. Pages without #dash-data do nothing.
(function () {
  var block = document.getElementById('dash-data');
  if (!block || typeof echarts === 'undefined') return;
  var data = JSON.parse(block.textContent);

  var PALETTE = ['#5b5bd6', '#00b3a4', '#f5b301', '#e0791a', '#5c7cfa', '#d6336c', '#74b816', '#862e9c'];

  function chart(id) {
    var el = document.getElementById(id);
    if (!el) return null;
    return echarts.init(el);
  }

  // Hourly timeline: stacked bars per tool.
  var hourly = chart('chart-hourly');
  if (hourly) {
    var tools = {};
    data.hourly.forEach(function (p) {
      Object.keys(p.Tools).forEach(function (t) { tools[t] = true; });
    });
    var toolNames = Object.keys(tools).sort();
    var hours = data.hourly.map(function (p) { return p.Hour + ':00'; });
    var series = toolNames.map(function (t, i) {
      return {
        name: t, type: 'bar', stack: 'total', barMaxWidth: 26,
        itemStyle: { color: PALETTE[i % PALETTE.length] },
        data: data.hourly.map(function (p) { return p.Tools[t] || 0; }),
      };
    });
    hourly.setOption({
      tooltip: { trigger: 'axis' },
      legend: { top: 0 },
      grid: { left: 60, right: 20, top: 30, bottom: 30 },
      xAxis: { type: 'category', data: hours },
      yAxis: { type: 'value' },
      series: series,
    });
  }

  // 30-day tokens + cost dual axis.
  var daily = chart('chart-daily');
  if (daily) {
    daily.setOption({
      tooltip: { trigger: 'axis' },
      legend: { top: 0 },
      grid: { left: 60, right: 60, top: 30, bottom: 30 },
      xAxis: { type: 'category', data: data.daily.map(function (d) { return d.Date.slice(5); }) },
      yAxis: [
        { type: 'value', name: 'tokens' },
        { type: 'value', name: 'USD', splitLine: { show: false } },
      ],
      series: [
        {
          name: 'tokens', type: 'bar', barMaxWidth: 22, itemStyle: { color: '#5b5bd6' },
          data: data.daily.map(function (d) { return d.Tokens; }),
        },
        {
          name: 'cost', type: 'line', yAxisIndex: 1, smooth: true, itemStyle: { color: '#f5b301' },
          data: data.daily.map(function (d) { return +d.Cost.toFixed(4); }),
        },
      ],
    });
  }

  function barOf(id, rows, color) {
    var c = chart(id);
    if (!c) return;
    c.setOption({
      tooltip: {},
      grid: { left: 110, right: 20, top: 10, bottom: 30 },
      xAxis: { type: 'value' },
      yAxis: { type: 'category', data: rows.map(function (r) { return r.Model; }).reverse() },
      series: [{
        type: 'bar', barMaxWidth: 18, itemStyle: { color: color, borderRadius: [0, 6, 6, 0] },
        data: rows.map(function (r) { return r.Tokens; }).reverse(),
      }],
    });
  }

  barOf('chart-tool', data.byTool, '#00b3a4');
  barOf('chart-model', data.byModel.slice(0, 8), '#5b5bd6');

  var compose = chart('chart-compose');
  if (compose) {
    compose.setOption({
      tooltip: { trigger: 'item' },
      series: [{
        type: 'pie', radius: ['40%', '70%'],
        label: { formatter: '{b}\n{d}%' },
        data: [
          { name: '输入', value: data.compose.Input, itemStyle: { color: '#5b5bd6' } },
          { name: '输出', value: data.compose.Output, itemStyle: { color: '#00b3a4' } },
          { name: '缓存读', value: data.compose.CacheRead, itemStyle: { color: '#f5b301' } },
          { name: '缓存写 5m', value: data.compose.Cache5m, itemStyle: { color: '#e0791a' } },
          { name: '缓存写 1h', value: data.compose.Cache1h, itemStyle: { color: '#d6336c' } },
        ].filter(function (d) { return d.value > 0; }),
      }],
    });
  }

  window.addEventListener('resize', function () {
    document.querySelectorAll('.chart').forEach(function (el) {
      var inst = echarts.getInstanceByDom(el);
      if (inst) inst.resize();
    });
  });
})();
