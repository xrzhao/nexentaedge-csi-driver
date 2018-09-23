package csi

import (
	"os"
	"strings"

	"github.com/Nexenta/nexentaedge-csi-driver/csi/nexentaedge"
	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
}

func (ns *nodeServer) NodeGetId(ctx context.Context, req *csi.NodeGetIdRequest) (*csi.NodeGetIdResponse, error) {
	log.Infof("NodeGetId req[%#v]", req)
	// Using default function
	log.Info("NodeGetId invoked")
	return ns.DefaultNodeServer.NodeGetId(ctx, req)
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	defer elapsed("NodeServer::NodePublishVolume")()
	log.Infof("NodeServer::NodePublishVolume req[%+v]", *req)
	nedge, err := nexentaedge.InitNexentaEdge("NodeServer::NodePublishVolume")
	if err != nil {
		log.Fatal("Failed to get NexentaEdge instance")
		return nil, err
	}

	//log.Info("NodeServer::NodePublishVolume:nedge : %+v", nedge)
	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	// Check arguments
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume id must be provided")
	}
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path must be provided")
	}

	volID, clusterData, err := nedge.GetClusterDataByVolumeID(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Can't get cluster information by volumeID:%s, Error:%s", volumeID, err)
	}
	//log.Infof("VolumeID: %s ClusterData: %+v", volumeID, clusterData)

	// find service to serve
	serviceData, err := clusterData.FindServiceDataByVolumeID(volID)

	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Can't find service data by VolumeID:%s Error:%s", volID, err)
	}
	log.Infof("Service %s found by volumeID %s", serviceData.Service, volumeID)

	nfsVolume, nfsEndpoint, err := serviceData.GetNFSVolumeAndEndpoint(volID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Can't find NFS Volume or endpoint by VolumeID:%s Error:%s", volID, err)
	}

	mounter := mount.New("")
	notMnt, err := mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		//log.Info("notMnt is False skipping")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	log.Infof("NodeServer::NodePublishVolume Publishing nfs volume %+v", nfsVolume)
	//log.Infof("NexentaEdge export %s endpoint is %s", volID.FullObjectPath(), nfsEndpoint)

	err = mounter.Mount(nfsEndpoint, targetPath, "nfs", nedge.GetClusterConfig().NfsMountOptionsArray)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.Infof("NodeServer::NodePublishVolume volumeID: %s, targetPath: %s, endpoint: %s", volID, targetPath, nfsEndpoint)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	defer elapsed("NodeServer::NodeUnpublishVolume")()
	log.Infof("NodeServer::NodeUnpublishVolume request[%+v]", *req)

	targetPath := req.GetTargetPath()
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "Targetpath not found")
		}
		return nil, status.Error(codes.Internal, err.Error())

	}
	if notMnt {
		return nil, status.Error(codes.NotFound, "Volume not mounted")
	}

	err = util.UnmountPath(req.GetTargetPath(), mount.New(""))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	//return &csi.NodeUnstageVolumeResponse{}, nil

	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	//return &csi.NodeStageVolumeResponse{}, nil
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
		},
	}, nil
}
