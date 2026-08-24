// Reference iOS PreviewHost for gophics's shell/mobile camera bridge.
//
// Wire once at startup:  bridge.setPreviewHost(GophicsPreview(bridge: bridge))
//
// This is the camera counterpart to GophicsMonitor: an open stream the Go side
// draws live, rather than a one-shot capture that ends with a result. A mirror
// or a scanner needs the former — the frame has to be current, not eventual.
//
// The app's Info.plist needs NSCameraUsageDescription; `gophics build` checks
// for it, and iOS terminates the process rather than prompting without it.

import AVFoundation
import Foundation
import Mirrormobile

final class GophicsPreview: NSObject {
    private let bridge: MobileBridge
    private let session = AVCaptureSession()

    // The camera's own queue. Frames must not be delivered on the main queue:
    // deliverPreviewFrame touches no app code, and routing it through the main
    // queue would add a frame of latency and jank the UI at 30fps.
    private let queue = DispatchQueue(label: "gophics.camera")

    private var activeReq = 0
    // rgba is reused across frames; Go copies out of it immediately.
    private var rgba = [UInt8]()

    init(bridge: MobileBridge) {
        self.bridge = bridge
    }

    private func main(_ f: @escaping () -> Void) { DispatchQueue.main.async(execute: f) }
}

extension GophicsPreview: MobilePreviewHostProtocol {
    func authorizeCamera(_ reqID: Int) {
        switch AVCaptureDevice.authorizationStatus(for: .video) {
        case .authorized:
            main { self.bridge.deliverPermission(reqID, granted: true) }
        case .notDetermined:
            AVCaptureDevice.requestAccess(for: .video) { ok in
                self.main { self.bridge.deliverPermission(reqID, granted: ok) }
            }
        default:
            main { self.bridge.deliverPermission(reqID, granted: false) }
        }
    }

    func startPreview(_ reqID: Int, facing: Int, width: Int) {
        // Ask first if the user has not been asked: PreviewHost's contract says
        // starting may prompt, and refusing outright when the answer was simply
        // never sought makes an app look broken on first run.
        switch AVCaptureDevice.authorizationStatus(for: .video) {
        case .authorized:
            break
        case .notDetermined:
            AVCaptureDevice.requestAccess(for: .video) { granted in
                guard granted else {
                    self.main { self.bridge.failPreview(reqID, msg: "camera permission denied") }
                    return
                }
                self.main { self.startPreview(reqID, facing: facing, width: width) }
            }
            return
        default:
            main { self.bridge.failPreview(reqID, msg: "camera permission denied") }
            return
        }
        queue.async { self.open(reqID, facing: facing, width: width) }
    }

    func stopPreview(_ reqID: Int) {
        queue.async {
            if self.session.isRunning { self.session.stopRunning() }
            self.session.inputs.forEach(self.session.removeInput)
            self.session.outputs.forEach(self.session.removeOutput)
            self.activeReq = 0
        }
    }

    private func open(_ reqID: Int, facing: Int, width: Int) {
        // Any camera beats none: a device with only a back camera should still
        // show something when the app asked for the front one.
        let want: AVCaptureDevice.Position = facing == 1 ? .back : .front
        let discovery = AVCaptureDevice.DiscoverySession(
            deviceTypes: [.builtInWideAngleCamera],
            mediaType: .video,
            position: .unspecified)
        let device = discovery.devices.first { $0.position == want } ?? discovery.devices.first
        guard let device, let input = try? AVCaptureDeviceInput(device: device) else {
            main { self.bridge.failPreview(reqID, msg: "no camera on this device") }
            return
        }

        session.beginConfiguration()
        // A preview does not need capture resolution, and the conversion below
        // is per-pixel: asking for more costs frames per second, not detail.
        session.sessionPreset = width > 1280 ? .hd1920x1080 : .vga640x480
        guard session.canAddInput(input) else {
            session.commitConfiguration()
            main { self.bridge.failPreview(reqID, msg: "the camera could not be opened") }
            return
        }
        session.addInput(input)

        let out = AVCaptureVideoDataOutput()
        // 32BGRA so every frame arrives in one known layout; the alternative is
        // handling whatever planar format the device prefers today.
        out.videoSettings = [kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32BGRA]
        // A preview wants the newest frame, not every frame.
        out.alwaysDiscardsLateVideoFrames = true
        out.setSampleBufferDelegate(self, queue: queue)
        guard session.canAddOutput(out) else {
            session.commitConfiguration()
            main { self.bridge.failPreview(reqID, msg: "the camera could not be opened") }
            return
        }
        session.addOutput(out)

        // Rotate to match the device, or the picture arrives sideways.
        //
        // The sensor delivers its own landscape orientation no matter how the
        // phone is held, so a portrait app that does not ask for a rotation
        // gets frames turned ninety degrees — which is exactly what it looked
        // like on a real phone, and could not be seen in the Simulator, which
        // has no camera at all.
        if let conn = out.connection(with: .video) {
            if #available(iOS 17.0, *) {
                if conn.isVideoRotationAngleSupported(90) {
                    conn.videoRotationAngle = 90
                }
            } else if conn.isVideoOrientationSupported {
                conn.videoOrientation = .portrait
            }
        }
        session.commitConfiguration()

        activeReq = reqID
        session.startRunning()
        main { self.bridge.deliverPreviewReady(reqID) }
    }
}

extension GophicsPreview: AVCaptureVideoDataOutputSampleBufferDelegate {
    func captureOutput(
        _ output: AVCaptureOutput,
        didOutput sampleBuffer: CMSampleBuffer,
        from connection: AVCaptureConnection
    ) {
        guard activeReq != 0, let pb = CMSampleBufferGetImageBuffer(sampleBuffer) else { return }
        CVPixelBufferLockBaseAddress(pb, .readOnly)
        defer { CVPixelBufferUnlockBaseAddress(pb, .readOnly) }
        guard let base = CVPixelBufferGetBaseAddress(pb) else { return }

        let w = CVPixelBufferGetWidth(pb)
        let h = CVPixelBufferGetHeight(pb)
        let rowBytes = CVPixelBufferGetBytesPerRow(pb)
        let src = base.assumingMemoryBound(to: UInt8.self)

        if rgba.count != w * h * 4 { rgba = [UInt8](repeating: 0, count: w * h * 4) }
        rgba.withUnsafeMutableBufferPointer { dst in
            guard let d = dst.baseAddress else { return }
            for y in 0..<h {
                let s = src + y * rowBytes
                let o = d + y * w * 4
                for x in stride(from: 0, to: w * 4, by: 4) {
                    // BGRA → RGBA; the alpha the camera reports is not meaningful.
                    o[x] = s[x + 2]
                    o[x + 1] = s[x + 1]
                    o[x + 2] = s[x]
                    o[x + 3] = 0xFF
                }
            }
        }
        // Camera queue on purpose — see the class comment.
        bridge.deliverPreviewFrame(activeReq, rgba: Data(rgba), w: w, h: h)
    }
}
