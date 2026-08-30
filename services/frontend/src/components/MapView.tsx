import React, { useEffect, useRef } from 'react';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import {
  LandmaskPolygon,
  MultiRouteResult,
  Point,
  RouteResult,
  WEATHER_MODELS,
  WeatherGridResponse,
  WeatherModelId,
} from '../types';

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
  onStartChange: (p: Point) => void;
  onDestChange: (p: Point) => void;
  placementMode: 'start' | 'dest';
  onPlacementModeChange: (mode: 'start' | 'dest') => void;
  routeResult: RouteResult | null;
  multiRouteResult?: MultiRouteResult | null;
  activeModel?: WeatherModelId;
  onSelectModel?: (modelId: WeatherModelId) => void;
  currentWaypointIndex: number;
  weatherGrid: WeatherGridResponse | null;
  landmaskPolygons: LandmaskPolygon[];
  showIsochrones: boolean;
  showWindGrid: boolean;
  showLandmask: boolean;
}

/**
 * Color mapping according to wind intensity specification:
 * 0 kts: Blue (29, 78, 216)
 * 10 kts: Green (16, 185, 129)
 * 20 kts: Yellow (250, 204, 21)
 * 30 kts: Orange (249, 115, 22)
 * 40 kts: Red (239, 68, 68)
 * 50+ kts: Violet (168, 85, 247)
 */
function getWindColorRGB(tws: number): [number, number, number, number] {
  const alpha = 0.92;
  if (tws <= 0) return [29, 78, 216, alpha];
  if (tws < 10) {
    const f = tws / 10;
    return [
      Math.round(29 + f * (16 - 29)),
      Math.round(78 + f * (185 - 78)),
      Math.round(216 + f * (129 - 216)),
      alpha,
    ];
  } else if (tws < 20) {
    const f = (tws - 10) / 10;
    return [
      Math.round(16 + f * (250 - 16)),
      Math.round(185 + f * (204 - 185)),
      Math.round(129 + f * (21 - 129)),
      alpha,
    ];
  } else if (tws < 30) {
    const f = (tws - 20) / 10;
    return [
      Math.round(250 + f * (249 - 250)),
      Math.round(204 + f * (115 - 204)),
      Math.round(21 + f * (22 - 21)),
      alpha,
    ];
  } else if (tws < 40) {
    const f = (tws - 30) / 10;
    return [
      Math.round(249 + f * (239 - 249)),
      Math.round(115 + f * (68 - 115)),
      Math.round(22 + f * (68 - 22)),
      alpha,
    ];
  } else if (tws < 50) {
    const f = (tws - 40) / 10;
    return [
      Math.round(239 + f * (168 - 239)),
      Math.round(68 + f * (85 - 68)),
      Math.round(68 + f * (247 - 68)),
      alpha,
    ];
  } else {
    const f = Math.min((tws - 50) / 20, 1.0);
    return [
      Math.round(168 + f * (139 - 168)),
      Math.round(85 + f * (92 - 85)),
      Math.round(247 + f * (246 - 247)),
      0.95,
    ];
  }
}

/**
 * Returns a significantly darker, high-contrast shade of the wind speed color for barbs.
 */
function getDarkWindBarbColor(tws: number): string {
  const [r, g, b] = getWindColorRGB(tws);
  const darkR = Math.round(r * 0.40);
  const darkG = Math.round(g * 0.40);
  const darkB = Math.round(b * 0.40);
  return `rgb(${darkR}, ${darkG}, ${darkB})`;
}

/**
 * Generates an SVG string for a classical meteorological wind barb colored by a darker version of wind speed.
 */
function createClassicalWindBarbSVG(twsKts: number, twdDeg: number): string {
  const roundedSpd = Math.round(twsKts / 5) * 5;
  const flags = Math.floor(roundedSpd / 50);
  let rem = roundedSpd % 50;
  const fullBarbs = Math.floor(rem / 10);
  rem = rem % 10;
  const halfBarbs = Math.floor(rem / 5);
  const barbColor = getDarkWindBarbColor(twsKts);

  let elements = '';
  if (roundedSpd < 2.5) {
    elements = `<circle cx="16" cy="16" r="3.5" fill="none" stroke="${barbColor}" stroke-width="2.2" opacity="0.98"/>`;
    return `
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32" style="filter: drop-shadow(0 0 1.2px rgba(255,255,255,0.65)) drop-shadow(0 1px 2px rgba(0,0,0,0.85));">
        ${elements}
      </svg>
    `;
  }

  // Staff pointing FROM wind direction
  elements += `<line x1="16" y1="16" x2="16" y2="3.5" stroke="${barbColor}" stroke-width="2.2" stroke-linecap="round" opacity="0.98"/>`;

  let yPos = 3.5;
  const barbSpacing = 2.8;

  for (let i = 0; i < flags; i++) {
    elements += `<polygon points="16,${yPos} 23,${yPos + 2.0} 16,${yPos + 4.0}" fill="${barbColor}" stroke="${barbColor}" stroke-width="0.8" opacity="0.98"/>`;
    yPos += 4.5;
  }
  for (let i = 0; i < fullBarbs; i++) {
    elements += `<line x1="16" y1="${yPos}" x2="22.5" y2="${yPos - 2.0}" stroke="${barbColor}" stroke-width="2.2" stroke-linecap="round" opacity="0.98"/>`;
    yPos += barbSpacing;
  }
  for (let i = 0; i < halfBarbs; i++) {
    elements += `<line x1="16" y1="${yPos}" x2="19.5" y2="${yPos - 1.2}" stroke="${barbColor}" stroke-width="2.2" stroke-linecap="round" opacity="0.98"/>`;
    yPos += barbSpacing;
  }

  return `
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32" style="transform: rotate(${twdDeg}deg); transform-origin: 16px 16px; filter: drop-shadow(0 0 1.2px rgba(255,255,255,0.65)) drop-shadow(0 1px 2px rgba(0,0,0,0.85));">
      ${elements}
    </svg>
  `;
}

function renderWindHeatmapCanvas(weatherGrid: WeatherGridResponse): HTMLCanvasElement {
  const { grid } = weatherGrid;
  const nLat = grid.length;
  const nLon = grid[0]?.length || 0;

  const canvas = document.createElement('canvas');
  canvas.width = Math.max(nLon * 16, 256);
  canvas.height = Math.max(nLat * 16, 256);

  const ctx = canvas.getContext('2d');
  if (!ctx || nLat === 0 || nLon === 0) return canvas;

  const gridCanvas = document.createElement('canvas');
  gridCanvas.width = nLon;
  gridCanvas.height = nLat;
  const gridCtx = gridCanvas.getContext('2d');
  if (!gridCtx) return canvas;

  const imgData = gridCtx.createImageData(nLon, nLat);
  const data = imgData.data;

  for (let i = 0; i < nLat; i++) {
    const rowIdx = nLat - 1 - i;
    for (let j = 0; j < nLon; j++) {
      const wind = grid[rowIdx]?.[j];
      const pixelIdx = (i * nLon + j) * 4;
      if (wind) {
        const [r, g, b, a] = getWindColorRGB(wind.tws_kts);
        data[pixelIdx] = r;
        data[pixelIdx + 1] = g;
        data[pixelIdx + 2] = b;
        data[pixelIdx + 3] = Math.round(a * 255);
      } else {
        data[pixelIdx] = 0;
        data[pixelIdx + 1] = 0;
        data[pixelIdx + 2] = 0;
        data[pixelIdx + 3] = 0;
      }
    }
  }

  gridCtx.putImageData(imgData, 0, 0);

  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = 'high';
  ctx.drawImage(gridCanvas, 0, 0, canvas.width, canvas.height);

  return canvas;
}

export const MapView: React.FC<MapViewProps> = ({
  startPoint,
  destPoint,
  onStartChange,
  onDestChange,
  placementMode,
  onPlacementModeChange,
  routeResult,
  multiRouteResult,
  activeModel = 'gfs_0p25',
  onSelectModel,
  currentWaypointIndex,
  weatherGrid,
  landmaskPolygons,
  showIsochrones,
  showWindGrid,
  showLandmask,
}) => {
  const mapContainerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<L.Map | null>(null);

  const layersRef = useRef<{
    startMarker?: L.Marker;
    destMarker?: L.Marker;
    baselinePolyline?: L.Polyline;
    boatMarker?: L.Marker;
    isochroneGroup?: L.FeatureGroup;
    windGroup?: L.FeatureGroup;
    landmaskGroup?: L.FeatureGroup;
    multiRouteGroup?: L.FeatureGroup;
    windHeatmapOverlay?: L.ImageOverlay;
  }>({});

  const onStartChangeRef = useRef(onStartChange);
  const onDestChangeRef = useRef(onDestChange);
  onStartChangeRef.current = onStartChange;
  onDestChangeRef.current = onDestChange;

  // 1. Initialize Map
  useEffect(() => {
    if (!mapContainerRef.current || mapRef.current) return;

    const map = L.map(mapContainerRef.current, {
      center: [(startPoint.lat + destPoint.lat) / 2, (startPoint.lon + destPoint.lon) / 2],
      zoom: 5,
      zoomControl: true,
    });

    // Dark Matter tile layer
    L.tileLayer(
      'https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png',
      {
        attribution:
          '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>',
        subdomains: 'abcd',
        maxZoom: 19,
      }
    ).addTo(map);

    layersRef.current.isochroneGroup = L.featureGroup().addTo(map);
    layersRef.current.multiRouteGroup = L.featureGroup().addTo(map);
    layersRef.current.windGroup = L.featureGroup().addTo(map);
    layersRef.current.landmaskGroup = L.featureGroup().addTo(map);

    // Dedicated Leaflet pane for wind heatmap (between base tiles and overlays)
    map.createPane('windHeatmapPane');
    const windPane = map.getPane('windHeatmapPane');
    if (windPane) {
      windPane.style.zIndex = '250';
      windPane.style.pointerEvents = 'none';
    }

    // Map click handler for interactive waypoint placement
    map.on('click', (e: L.LeafletMouseEvent) => {
      const clickedLat = Number(e.latlng.lat.toFixed(4));
      const clickedLon = Number(e.latlng.lng.toFixed(4));

      if (placementMode === 'start') {
        onStartChangeRef.current?.({ lat: clickedLat, lon: clickedLon });
        onPlacementModeChange('dest');
      } else {
        onDestChangeRef.current?.({ lat: clickedLat, lon: clickedLon });
        onPlacementModeChange('start');
      }
    });

    mapRef.current = map;

    return () => {
      map.remove();
      mapRef.current = null;
    };
  }, []);

  // 2. Render Start & Destination Markers and Baseline
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    if (layersRef.current.startMarker) {
      layersRef.current.startMarker.remove();
    }
    if (layersRef.current.destMarker) {
      layersRef.current.destMarker.remove();
    }
    if (layersRef.current.baselinePolyline) {
      layersRef.current.baselinePolyline.remove();
    }

    const startIcon = L.divIcon({
      className: 'custom-pin-container',
      html: `
        <div class="pin-bubble-start">
          <span>⚓</span> START
        </div>
        <div class="pin-pointer"></div>
      `,
      iconSize: [68, 30],
      iconAnchor: [34, 30],
    });

    const destIcon = L.divIcon({
      className: 'custom-pin-container',
      html: `
        <div class="pin-bubble-dest">
          <span>🏁</span> FINISH
        </div>
        <div class="pin-pointer"></div>
      `,
      iconSize: [72, 30],
      iconAnchor: [36, 30],
    });

    const updateBaseline = (p1: [number, number], p2: [number, number]) => {
      if (layersRef.current.baselinePolyline) {
        layersRef.current.baselinePolyline.setLatLngs([p1, p2]);
      }
    };

    // Create Start Marker
    const startMarker = L.marker([startPoint.lat, startPoint.lon], {
      icon: startIcon,
      draggable: true,
      autoPan: true,
      zIndexOffset: 600,
    });

    startMarker.on('drag', (e) => {
      const latlng = (e.target as L.Marker).getLatLng();
      updateBaseline([latlng.lat, latlng.lng], [destPoint.lat, destPoint.lon]);
    });

    startMarker.on('dragend', (e) => {
      const latlng = (e.target as L.Marker).getLatLng();
      onStartChangeRef.current?.({
        lat: Number(latlng.lat.toFixed(4)),
        lon: Number(latlng.lng.toFixed(4)),
      });
    });

    startMarker.bindTooltip(
      `<b>Start Point</b><br/>Lat: ${startPoint.lat.toFixed(4)}°<br/>Lon: ${startPoint.lon.toFixed(4)}°<br/><span style="color:#10b981; font-size:10px;">✋ Drag to move</span>`,
      { direction: 'top', offset: [0, -26] }
    );
    startMarker.addTo(map);
    layersRef.current.startMarker = startMarker;

    // Create Destination Marker
    const destMarker = L.marker([destPoint.lat, destPoint.lon], {
      icon: destIcon,
      draggable: true,
      autoPan: true,
      zIndexOffset: 600,
    });

    destMarker.on('drag', (e) => {
      const latlng = (e.target as L.Marker).getLatLng();
      updateBaseline([startPoint.lat, startPoint.lon], [latlng.lat, latlng.lng]);
    });

    destMarker.on('dragend', (e) => {
      const latlng = (e.target as L.Marker).getLatLng();
      onDestChangeRef.current?.({
        lat: Number(latlng.lat.toFixed(4)),
        lon: Number(latlng.lng.toFixed(4)),
      });
    });

    destMarker.bindTooltip(
      `<b>Finish Point</b><br/>Lat: ${destPoint.lat.toFixed(4)}°<br/>Lon: ${destPoint.lon.toFixed(4)}°<br/><span style="color:#ef4444; font-size:10px;">✋ Drag to move</span>`,
      { direction: 'top', offset: [0, -26] }
    );
    destMarker.addTo(map);
    layersRef.current.destMarker = destMarker;

    // Draw connecting baseline if no route calculated
    if (!routeResult) {
      layersRef.current.baselinePolyline = L.polyline(
        [
          [startPoint.lat, startPoint.lon],
          [destPoint.lat, destPoint.lon],
        ],
        {
          color: '#38bdf8',
          weight: 1.8,
          dashArray: '5, 6',
          opacity: 0.65,
        }
      ).addTo(map);
    }
  }, [startPoint, destPoint, routeResult]);

  // 3. Render Wind Heatmap Background & Wind Barbs for Active Model
  useEffect(() => {
    const map = mapRef.current;
    const group = layersRef.current.windGroup;
    if (!map || !group) return;

    group.clearLayers();

    // Clean up previous overlay to avoid setUrl/setBounds race conditions
    if (layersRef.current.windHeatmapOverlay) {
      layersRef.current.windHeatmapOverlay.remove();
      layersRef.current.windHeatmapOverlay = undefined;
    }

    if (!showWindGrid || !weatherGrid || weatherGrid.grid.length === 0) {
      return;
    }

    const { grid, min_lat, max_lat, min_lon, max_lon, lat_step, lon_step } = weatherGrid;

    // A. Render Smooth Wind Intensity Heatmap Overlay into dedicated pane
    const canvas = renderWindHeatmapCanvas(weatherGrid);
    const bounds = L.latLngBounds([
      [Math.min(min_lat, max_lat), Math.min(min_lon, max_lon)],
      [Math.max(min_lat, max_lat), Math.max(min_lon, max_lon)],
    ]);

    layersRef.current.windHeatmapOverlay = L.imageOverlay(canvas.toDataURL(), bounds, {
      opacity: 0.90,
      interactive: false,
      pane: 'windHeatmapPane',
    }).addTo(map);

    // B. Render Meteorological Wind Barbs (colored by dark wind speed shade)
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

  // 4. Render Multi-Model Routes and Active Isochrones
  useEffect(() => {
    const map = mapRef.current;
    const group = layersRef.current.multiRouteGroup;
    const isoGroup = layersRef.current.isochroneGroup;
    if (!map || !group || !isoGroup) return;

    group.clearLayers();
    isoGroup.clearLayers();

    // Prepare dictionary of routes to display
    const routesToDisplay: Record<string, RouteResult> = {};
    if (multiRouteResult && Object.keys(multiRouteResult).length > 0) {
      Object.entries(multiRouteResult).forEach(([mId, r]) => {
        if (r) routesToDisplay[mId] = r;
      });
    } else if (routeResult) {
      routesToDisplay[activeModel] = routeResult;
    }

    if (Object.keys(routesToDisplay).length === 0) return;

    const allCoords: [number, number][] = [];

    // Render each model route track
    Object.entries(routesToDisplay).forEach(([mId, r]) => {
      const meta = WEATHER_MODELS[mId] || {
        id: mId,
        name: mId.toUpperCase(),
        shortName: mId,
        color: '#0284c7',
        lightColor: '#38bdf8',
      };

      const isActive = mId === activeModel;
      const routeCoords = r.waypoints.map((wp) => [wp.lat, wp.lon] as [number, number]);
      allCoords.push(...routeCoords);

      // A. Draw Route Polyline
      const polyline = L.polyline(routeCoords, {
        color: meta.color,
        weight: isActive ? 4.8 : 3.0,
        opacity: isActive ? 0.95 : 0.70,
        dashArray: isActive ? undefined : '6, 6',
        lineCap: 'round',
        lineJoin: 'round',
      });

      if (isActive) {
        polyline.bringToFront();
      }

      // Interactive Tooltip & Click-to-Select
      const statusLabel = isActive
        ? `<span style="color:${meta.lightColor}; font-weight:700;">★ ACTIVE MODEL</span>`
        : `<span style="color:${meta.lightColor}; font-size:10px;">👉 Click to set active</span>`;

      polyline.bindTooltip(
        `<div style="font-family: var(--font-sans); font-size: 11px; line-height: 1.4;">
           <b style="color:${meta.lightColor};">${meta.name}</b><br/>
           Duration: <b>${r.total_duration_hours.toFixed(1)} hrs</b> (${(r.total_duration_hours / 24).toFixed(1)} days)<br/>
           Distance: <b>${r.total_distance_nm.toFixed(1)} NM</b> (Avg ${r.average_speed_kts.toFixed(1)} kts)<br/>
           Max Wind: <b>${r.max_wind_kts.toFixed(1)} kts</b> | Tacks: ${r.total_tacks}<br/>
           ${statusLabel}
         </div>`,
        { sticky: true }
      );

      if (!isActive && onSelectModel) {
        polyline.on('click', () => {
          onSelectModel(mId);
        });
      }

      group.addLayer(polyline);

      // B. Draw Isochrones for Active Route (if toggled)
      if (isActive && showIsochrones && r.isochrones) {
        r.isochrones.forEach((wave, idx) => {
          if (idx % 2 === 0 && wave.points.length > 1) {
            const latlngs = wave.points.map((p) => [p.lat, p.lon] as [number, number]);
            L.polyline(latlngs, {
              color: meta.lightColor,
              weight: 1.2,
              opacity: 0.45,
              dashArray: '3, 4',
            }).addTo(isoGroup);
          }
        });
      }
    });

    // Fit map bounds to show all tracks
    if (allCoords.length > 0) {
      const bounds = L.latLngBounds([
        [startPoint.lat, startPoint.lon],
        [destPoint.lat, destPoint.lon],
        ...allCoords,
      ]);
      map.fitBounds(bounds, { padding: [50, 50], maxZoom: 8 });
    }
  }, [multiRouteResult, routeResult, activeModel, showIsochrones, startPoint, destPoint, onSelectModel]);

  // 5. Render Animated Boat Marker for Active Route
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    if (layersRef.current.boatMarker) {
      layersRef.current.boatMarker.remove();
    }

    if (!routeResult || !routeResult.waypoints || routeResult.waypoints.length === 0) return;

    const meta = WEATHER_MODELS[activeModel] || {
      color: '#0284c7',
      lightColor: '#38bdf8',
      shortName: activeModel,
    };

    const wp = routeResult.waypoints[currentWaypointIndex] || routeResult.waypoints[0];
    const boatIcon = L.divIcon({
      className: 'boat-marker',
      html: `
        <div style="transform: rotate(${wp.heading_deg}deg); transform-origin: center; display:flex; align-items:center; justify-content:center;">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="${meta.color}" stroke="#ffffff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="filter: drop-shadow(0 0 8px ${meta.color});">
            <polygon points="12 2 19 21 12 17 5 21 12 2"></polygon>
          </svg>
        </div>
      `,
      iconSize: [32, 32],
      iconAnchor: [16, 16],
    });

    layersRef.current.boatMarker = L.marker([wp.lat, wp.lon], { icon: boatIcon, zIndexOffset: 1000 })
      .bindPopup(`
        <div style="font-family: var(--font-sans); font-size: 12px; line-height: 1.4;">
          <b>Boat: ${routeResult.boat_name}</b> <span style="color:${meta.lightColor}; font-size:10px;">[${meta.shortName}]</span><br/>
          Speed: <b>${wp.boat_speed_kts.toFixed(2)} kts</b><br/>
          Heading: ${wp.heading_deg.toFixed(1)}°<br/>
          TWS: ${wp.tws_kts.toFixed(1)} kts | TWA: ${wp.twa_deg.toFixed(1)}°<br/>
          Dist to dest: ${wp.distance_to_dest_nm.toFixed(1)} NM
        </div>
      `)
      .addTo(map);
  }, [routeResult, currentWaypointIndex, activeModel]);

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
        color: '#000000',
        weight: 1,
        opacity: 0.95,
        fillColor: '#6b7280',
        fillOpacity: 0.45,
      });

      polygonLayer.bindTooltip(
        `<div style="font-family: var(--font-sans); font-size: 11px;">
           <b style="color: #e2e8f0;">🛡️ ${poly.name}</b><br/>
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
