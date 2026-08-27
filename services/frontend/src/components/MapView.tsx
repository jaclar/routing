import React, { useEffect, useRef } from 'react';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { LandmaskPolygon, Point, RouteResult, WeatherGridResponse } from '../types';

// Fix Leaflet default marker icons for Webpack/Vite
delete (L.Icon.Default.prototype as any)._getIconUrl;
L.Icon.Default.mergeOptions({
  iconRetinaUrl: 'https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon-2x.png',
  iconUrl: 'https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon.png',
  shadowUrl: 'https://unpkg.com/leaflet@1.9.4/dist/images/marker-shadow.png',
});

interface MapViewProps {
  startPoint: Point;
  destPoint: Point;
  routeResult: RouteResult | null;
  currentWaypointIndex: number;
  weatherGrid: WeatherGridResponse | null;
  landmaskPolygons: LandmaskPolygon[];
  showIsochrones: boolean;
  showWindGrid: boolean;
  showLandmask: boolean;
}

/**
 * Color mapping according to user specification:
 * 0 kts: Blue
 * 10 kts: Green
 * 20 kts: Yellow
 * 30 kts: Orange
 * 40 kts: Red
 * 50+ kts: Violet
 */
function getWindColorRGB(tws: number): [number, number, number, number] {
  const alpha = 0.78; // Increased intensity
  if (tws <= 0) return [29, 78, 216, alpha]; // 0 kt Blue
  if (tws < 10) {
    const f = tws / 10;
    // Blue (29, 78, 216) -> Green (16, 185, 129)
    return [
      Math.round(29 + f * (16 - 29)),
      Math.round(78 + f * (185 - 78)),
      Math.round(216 + f * (129 - 216)),
      alpha,
    ];
  } else if (tws < 20) {
    const f = (tws - 10) / 10;
    // Green (16, 185, 129) -> Yellow (250, 204, 21)
    return [
      Math.round(16 + f * (250 - 16)),
      Math.round(185 + f * (204 - 185)),
      Math.round(129 + f * (21 - 129)),
      alpha,
    ];
  } else if (tws < 30) {
    const f = (tws - 20) / 10;
    // Yellow (250, 204, 21) -> Orange (249, 115, 22)
    return [
      Math.round(250 + f * (249 - 250)),
      Math.round(204 + f * (115 - 204)),
      Math.round(21 + f * (22 - 21)),
      alpha,
    ];
  } else if (tws < 40) {
    const f = (tws - 30) / 10;
    // Orange (249, 115, 22) -> Red (239, 68, 68)
    return [
      Math.round(249 + f * (239 - 249)),
      Math.round(115 + f * (68 - 115)),
      Math.round(22 + f * (68 - 22)),
      alpha,
    ];
  } else if (tws < 50) {
    const f = (tws - 40) / 10;
    // Red (239, 68, 68) -> Violet (168, 85, 247)
    return [
      Math.round(239 + f * (168 - 239)),
      Math.round(68 + f * (85 - 68)),
      Math.round(68 + f * (247 - 68)),
      alpha,
    ];
  } else {
    const f = Math.min((tws - 50) / 20, 1.0);
    // Violet (168, 85, 247) -> Deep Violet (139, 92, 246)
    return [
      Math.round(168 + f * (139 - 168)),
      Math.round(85 + f * (92 - 85)),
      Math.round(247 + f * (246 - 247)),
      alpha + 0.05,
    ];
  }
}

/**
 * Generates an SVG string for a classical meteorological wind barb.
 * - Calm (< 2.5 kts): small open circle
 * - 50 kts: filled pennant (triangle)
 * - 10 kts: full barb line
 * - 5 kts: half barb line
 * - Oriented such that the shaft points in the direction the wind is blowing FROM (standard meteorological convention).
 */
function createClassicalWindBarbSVG(tws: number, twd: number): string {
  const speed = Math.round(tws / 5) * 5;

  if (speed < 3) {
    return `
      <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; width:32px; height:32px;">
        <svg width="24" height="24" viewBox="0 0 32 32">
          <circle cx="16" cy="16" r="3.5" fill="none" stroke="#ffffff" stroke-width="2" style="filter: drop-shadow(0 0 2px rgba(0,0,0,0.9));" />
        </svg>
        <span style="font-size:8px; font-weight:700; color:#f8fafc; margin-top:-2px; text-shadow:0 0 3px #000, 0 0 5px #000;">
          ${Math.round(tws)}
        </span>
      </div>
    `;
  }

  const num50 = Math.floor(speed / 50);
  const rem50 = speed % 50;
  const num10 = Math.floor(rem50 / 10);
  const num5 = Math.floor((rem50 % 10) / 5);

  const barbLines: string[] = [];
  let y = 3;
  const staffX = 16;
  const spacing = 3.5;

  // 50-kt flags (pennants)
  for (let k = 0; k < num50; k++) {
    barbLines.push(`
      <polygon points="${staffX},${y} ${staffX + 8.5},${y + 2.5} ${staffX},${y + 5}" fill="#ffffff" stroke="#090e17" stroke-width="0.7" />
    `);
    y += 5.5;
  }

  // 10-kt full barbs
  for (let k = 0; k < num10; k++) {
    barbLines.push(`
      <line x1="${staffX}" y1="${y}" x2="${staffX + 8.5}" y2="${y - 3.5}" stroke="#ffffff" stroke-width="2" stroke-linecap="round" />
    `);
    y += spacing;
  }

  // 5-kt half barb
  if (num5 > 0) {
    const y5 = num50 === 0 && num10 === 0 ? y + spacing : y;
    barbLines.push(`
      <line x1="${staffX}" y1="${y5}" x2="${staffX + 4.8}" y2="${y5 - 2}" stroke="#ffffff" stroke-width="2" stroke-linecap="round" />
    `);
  }

  return `
    <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; width:34px; height:34px;">
      <svg width="28" height="28" viewBox="0 0 32 32" style="transform: rotate(${twd}deg); transform-origin: 16px 25px; filter: drop-shadow(0 0 2.5px rgba(0,0,0,0.95));">
        <!-- Main barb shaft -->
        <line x1="16" y1="25" x2="16" y2="3" stroke="#ffffff" stroke-width="2" stroke-linecap="round" />
        <!-- Station anchor dot -->
        <circle cx="16" cy="25" r="1.8" fill="#ffffff" stroke="#090e17" stroke-width="0.5" />
        <!-- Pennants & barbs -->
        ${barbLines.join('\n')}
      </svg>
      <span style="font-size:8.5px; font-weight:700; color:#ffffff; margin-top:-3px; text-shadow:0 0 3px #000, 0 0 5px #000;">
        ${Math.round(tws)}
      </span>
    </div>
  `;
}

/**
 * Renders a smooth bilinearly-interpolated wind intensity heatmap canvas.
 */
function renderWindHeatmapCanvas(weatherGrid: WeatherGridResponse): HTMLCanvasElement {
  const { grid } = weatherGrid;
  const nLat = grid.length;
  const nLon = grid[0]?.length || 0;

  const targetW = 256;
  const targetH = 256;
  const canvas = document.createElement('canvas');
  canvas.width = targetW;
  canvas.height = targetH;
  const ctx = canvas.getContext('2d');
  if (!ctx || nLat === 0 || nLon === 0) return canvas;

  const imgData = ctx.createImageData(targetW, targetH);
  const data = imgData.data;

  for (let py = 0; py < targetH; py++) {
    // py=0 is top (max_lat), py=targetH-1 is bottom (min_lat)
    const latFrac = 1 - py / (targetH - 1);
    const gridY = latFrac * (nLat - 1);
    const y0 = Math.floor(gridY);
    const y1 = Math.min(y0 + 1, nLat - 1);
    const fy = gridY - y0;

    for (let px = 0; px < targetW; px++) {
      const lonFrac = px / (targetW - 1);
      const gridX = lonFrac * (nLon - 1);
      const x0 = Math.floor(gridX);
      const x1 = Math.min(x0 + 1, nLon - 1);
      const fx = gridX - x0;

      const s00 = grid[y0]?.[x0]?.tws_kts ?? 0;
      const s10 = grid[y0]?.[x1]?.tws_kts ?? 0;
      const s01 = grid[y1]?.[x0]?.tws_kts ?? 0;
      const s11 = grid[y1]?.[x1]?.tws_kts ?? 0;

      const sTop = s00 * (1 - fx) + s10 * fx;
      const sBot = s01 * (1 - fx) + s11 * fx;
      const tws = sTop * (1 - fy) + sBot * fy;

      const [r, g, b, a] = getWindColorRGB(tws);

      const pIdx = (py * targetW + px) * 4;
      data[pIdx] = r;
      data[pIdx + 1] = g;
      data[pIdx + 2] = b;
      data[pIdx + 3] = Math.round(a * 255);
    }
  }

  ctx.putImageData(imgData, 0, 0);
  return canvas;
}

export const MapView: React.FC<MapViewProps> = ({
  startPoint,
  destPoint,
  routeResult,
  currentWaypointIndex,
  weatherGrid,
  landmaskPolygons,
  showIsochrones,
  showWindGrid,
  showLandmask,
}) => {
  const mapContainerRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<L.Map | null>(null);
  const layersRef = useRef<{
    startMarker?: L.Marker;
    destMarker?: L.Marker;
    boatMarker?: L.Marker;
    routePolyline?: L.Polyline;
    isochroneGroup?: L.LayerGroup;
    windGroup?: L.LayerGroup;
    windHeatmapOverlay?: L.ImageOverlay;
    landmaskGroup?: L.LayerGroup;
  }>({});

  // 1. Initialize Map
  useEffect(() => {
    if (!mapContainerRef.current || mapRef.current) return;

    const map = L.map(mapContainerRef.current, {
      center: [(startPoint.lat + destPoint.lat) / 2, (startPoint.lon + destPoint.lon) / 2],
      zoom: 4,
      zoomControl: false,
      preferCanvas: true,
    });

    L.control.zoom({ position: 'topright' }).addTo(map);

    // Nautical CartoDB Voyager Base Layer
    L.tileLayer('https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png', {
      attribution: '&copy; CARTO &copy; OpenStreetMap contributors',
      maxZoom: 19,
      subdomains: 'abcd',
    }).addTo(map);

    // OpenSeaMap Nautical Seamarks overlay
    L.tileLayer('https://tiles.openseamap.org/seamark/{z}/{x}/{y}.png', {
      attribution: 'Map data &copy; OpenSeaMap',
      opacity: 0.7,
      maxZoom: 18,
    }).addTo(map);

    layersRef.current.landmaskGroup = L.layerGroup().addTo(map);
    layersRef.current.isochroneGroup = L.layerGroup().addTo(map);
    layersRef.current.windGroup = L.layerGroup().addTo(map);

    mapRef.current = map;

    // Trigger invalidateSize to force Leaflet layout inside flexbox container
    setTimeout(() => {
      if (mapRef.current) {
        mapRef.current.invalidateSize();
      }
    }, 100);

    return () => {
      map.remove();
      mapRef.current = null;
    };
  }, []);

  // 2. Render Start & Destination Markers
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    if (layersRef.current.startMarker) {
      layersRef.current.startMarker.remove();
    }
    if (layersRef.current.destMarker) {
      layersRef.current.destMarker.remove();
    }

    const startIcon = L.divIcon({
      className: 'custom-pin-start',
      html: `<div style="background:#10b981; color:#fff; font-weight:bold; border-radius:50%; width:24px; height:24px; display:flex; align-items:center; justify-content:center; border:2px solid #fff; box-shadow:0 0 8px rgba(16,185,129,0.8);">S</div>`,
      iconSize: [24, 24],
      iconAnchor: [12, 12],
    });

    const destIcon = L.divIcon({
      className: 'custom-pin-dest',
      html: `<div style="background:#ef4444; color:#fff; font-weight:bold; border-radius:50%; width:24px; height:24px; display:flex; align-items:center; justify-content:center; border:2px solid #fff; box-shadow:0 0 8px rgba(239,68,68,0.8);">D</div>`,
      iconSize: [24, 24],
      iconAnchor: [12, 12],
    });

    layersRef.current.startMarker = L.marker([startPoint.lat, startPoint.lon], { icon: startIcon })
      .bindPopup(`<b>Start Point</b><br/>Lat: ${startPoint.lat.toFixed(3)}<br/>Lon: ${startPoint.lon.toFixed(3)}`)
      .addTo(map);

    layersRef.current.destMarker = L.marker([destPoint.lat, destPoint.lon], { icon: destIcon })
      .bindPopup(`<b>Destination Point</b><br/>Lat: ${destPoint.lat.toFixed(3)}<br/>Lon: ${destPoint.lon.toFixed(3)}`)
      .addTo(map);

    if (!routeResult) {
      const bounds = L.latLngBounds([
        [startPoint.lat, startPoint.lon],
        [destPoint.lat, destPoint.lon],
      ]).pad(0.4);
      map.fitBounds(bounds, { maxZoom: 8, animate: true });
    }
  }, [startPoint, destPoint]);

  // 3. Render Wind Heatmap Background & Classical Wind Barbs
  useEffect(() => {
    const map = mapRef.current;
    const group = layersRef.current.windGroup;
    if (!map || !group) return;

    group.clearLayers();

    if (!showWindGrid || !weatherGrid || weatherGrid.grid.length === 0) {
      if (layersRef.current.windHeatmapOverlay) {
        layersRef.current.windHeatmapOverlay.remove();
        layersRef.current.windHeatmapOverlay = undefined;
      }
      return;
    }

    const { grid, min_lat, max_lat, min_lon, max_lon, lat_step, lon_step } = weatherGrid;

    // A. Render Smooth Wind Intensity Heatmap Overlay (0kt Blue, 15kt Yellow, 30kt Orange, 45+kt Violet)
    const canvas = renderWindHeatmapCanvas(weatherGrid);
    const bounds = L.latLngBounds([
      [min_lat, min_lon],
      [max_lat, max_lon],
    ]);

    if (layersRef.current.windHeatmapOverlay) {
      layersRef.current.windHeatmapOverlay.setUrl(canvas.toDataURL());
      layersRef.current.windHeatmapOverlay.setBounds(bounds);
    } else {
      layersRef.current.windHeatmapOverlay = L.imageOverlay(canvas.toDataURL(), bounds, {
        opacity: 0.75,
        interactive: false,
        zIndex: 200,
      }).addTo(map);
    }

    // B. Render Classical Meteorological Wind Barbs
    for (let i = 0; i < grid.length; i += 1) {
      const lat = min_lat + i * lat_step;
      for (let j = 0; j < grid[i].length; j += 1) {
        const lon = min_lon + j * lon_step;
        const wind = grid[i][j];
        if (!wind) continue;

        const barbHtml = createClassicalWindBarbSVG(wind.tws_kts, wind.twd_deg);
        const barbIcon = L.divIcon({
          className: 'classical-wind-barb',
          html: barbHtml,
          iconSize: [34, 34],
          iconAnchor: [17, 17],
        });

        L.marker([lat, lon], { icon: barbIcon, interactive: false }).addTo(group);
      }
    }
  }, [weatherGrid, showWindGrid]);

  // 4. Render Route and Isochrones
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    if (layersRef.current.routePolyline) {
      layersRef.current.routePolyline.remove();
    }
    if (layersRef.current.isochroneGroup) {
      layersRef.current.isochroneGroup.clearLayers();
    }

    if (!routeResult) return;

    // A. Render Isochrone Waves
    if (showIsochrones && routeResult.isochrones && layersRef.current.isochroneGroup) {
      routeResult.isochrones.forEach((wave, idx) => {
        if (idx % 2 === 0 && wave.points.length > 1) {
          const latlngs = wave.points.map((p) => [p.lat, p.lon] as [number, number]);
          L.polyline(latlngs, {
            color: '#38bdf8',
            weight: 1.2,
            opacity: 0.45,
            dashArray: '3, 4',
          }).addTo(layersRef.current.isochroneGroup!);
        }
      });
    }

    // B. Render Optimal Route Polyline
    const routeCoords = routeResult.waypoints.map((wp) => [wp.lat, wp.lon] as [number, number]);
    const routePoly = L.polyline(routeCoords, {
      color: '#0284c7',
      weight: 4.5,
      opacity: 0.95,
      lineCap: 'round',
      lineJoin: 'round',
    }).addTo(map);

    layersRef.current.routePolyline = routePoly;

    // Fit map bounds to show full route
    const bounds = L.latLngBounds([
      [startPoint.lat, startPoint.lon],
      [destPoint.lat, destPoint.lon],
      ...routeCoords,
    ]);
    map.fitBounds(bounds, { padding: [50, 50], maxZoom: 8 });
  }, [routeResult, showIsochrones, startPoint, destPoint]);

  // 5. Render Animated Boat Marker at current Waypoint
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    if (layersRef.current.boatMarker) {
      layersRef.current.boatMarker.remove();
    }

    if (!routeResult || !routeResult.waypoints || routeResult.waypoints.length === 0) return;

    const wp = routeResult.waypoints[currentWaypointIndex] || routeResult.waypoints[0];
    const boatIcon = L.divIcon({
      className: 'boat-marker',
      html: `
        <div style="transform: rotate(${wp.heading_deg}deg); transform-origin: center; display:flex; align-items:center; justify-content:center;">
          <svg width="30" height="30" viewBox="0 0 24 24" fill="#0284c7" stroke="#ffffff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="filter: drop-shadow(0 0 8px rgba(2,132,199,0.9));">
            <polygon points="12 2 19 21 12 17 5 21 12 2"></polygon>
          </svg>
        </div>
      `,
      iconSize: [30, 30],
      iconAnchor: [15, 15],
    });

    layersRef.current.boatMarker = L.marker([wp.lat, wp.lon], { icon: boatIcon, zIndexOffset: 1000 })
      .bindPopup(`
        <b>Boat: ${routeResult.boat_name}</b><br/>
        Speed: <b>${wp.boat_speed_kts.toFixed(2)} kts</b><br/>
        Heading: ${wp.heading_deg.toFixed(1)}°<br/>
        TWS: ${wp.tws_kts.toFixed(1)} kts | TWA: ${wp.twa_deg.toFixed(1)}°<br/>
        Dist to dest: ${wp.distance_to_dest_nm.toFixed(1)} NM
      `)
      .addTo(map);
  }, [routeResult, currentWaypointIndex]);

  // 6. Render Landmass Collision Polygons Layer
  useEffect(() => {
    const map = mapRef.current;
    const group = layersRef.current.landmaskGroup;
    if (!map || !group) return;

    group.clearLayers();
    if (!showLandmask || !landmaskPolygons || landmaskPolygons.length === 0) return;

    const canvasRenderer = L.canvas({ padding: 0.5 });

    landmaskPolygons.forEach((poly) => {
      if (!poly.vertices || poly.vertices.length < 3) return;
      const latLngs = poly.vertices.map((v) => [v.lat, v.lon] as [number, number]);

      const polygonLayer = L.polygon(latLngs, {
        renderer: canvasRenderer,
        color: '#f59e0b',          // Amber warning stroke
        weight: 2,
        dashArray: '5, 5',
        fillColor: '#ef4444',      // Coral red collision zone
        fillOpacity: 0.22,
      });

      polygonLayer.bindTooltip(
        `<div style="font-family: var(--font-sans); font-size: 11px;">
           <b style="color: #f59e0b;">🛡️ ${poly.name}</b><br/>
           <span style="color: #94a3b8;">GSHHG Land Boundary (${poly.vertices.length} vertices)</span>
         </div>`,
        { sticky: true }
      );

      group.addLayer(polygonLayer);
    });
  }, [landmaskPolygons, showLandmask]);

  return <div ref={mapContainerRef} className="map-view-container" />;
};

export default MapView;
