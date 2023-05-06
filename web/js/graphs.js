var graphId = 1;

function getGraphBaseConfig() {
    let cfg = {
        chart: {
            type: null,
            zoomType: 'x',
            animation: Highcharts.svg, // don't animate in old IE
            marginRight: 10,
        },
        time: {
            useUTC: false
        },
        title: {
            text: null
        },
        xAxis: {
            type: 'datetime',
            tickPixelInterval: 1000,
        },
        yAxis: {
            title: {
                text: 'Value'
            },
            plotLines: [{
                value: 0,
                width: 1,
                color: '#808080'
            }]
        },
        tooltip: {
            headerFormat: '<b>{series.name}</b><br/>',
            pointFormat: '{point.x:%Y-%m-%d %H:%M:%S}<br/>{point.y:.2f}'
        },
        series: [{
            name: null,
            data: []
        }]
    };
    return cfg;
}




function getGraphConfig(type, title) {
    let baseCfg = getGraphBaseConfig();
    switch (type) {
        case 'linegraph':
            baseCfg.chart.type = 'line';
            break;
        default:
            console.error('Not supported graph type ' + type)
    }
    baseCfg.title.text = title;
    baseCfg.series[0].name = title;
    return baseCfg;

}


function createGraph(data) {
    let container = $('<div class="graph" id="chart-' + graphId + '" />').appendTo('#container');
    let chartObj = Highcharts.chart('chart-' + graphId, getGraphConfig(data.Type, data.Name));
    graphId++;
    return chartObj;
}